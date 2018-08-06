// Copyright © 2018 The wormhole-connector authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"

	"github.com/kinvolk/wormhole-connector/internal/connection"
	"github.com/kinvolk/wormhole-connector/internal/header"
	"github.com/kinvolk/wormhole-connector/internal/http2error"
	"github.com/kinvolk/wormhole-connector/internal/streamio"
	"github.com/kinvolk/wormhole-connector/internal/tlsutil"
	"github.com/kinvolk/wormhole-connector/internal/tunnel"
)

var (
	logger     = log.WithFields(log.Fields{"component": "dispatcher"})
	bufferPool sync.Pool
	client     *http.Client

	flagConnectorServer = flag.String("connector-server", "https://localhost:8080", "Address of the Wormhole Connector. Required.")
	flagLocalAddr       = flag.String("local-addr", "localhost:4430", "TLS address to listen on ('host:port' or ':port'). Required.")
	flagTrustCAFile     = flag.String("trust-ca-file", "", "Path to a custom CA file for the Wormhole Connector address")
	flagInsecure        = flag.Bool("insecure", false, "Trust any CA for the Wormhole Connector")
)

func handleIncomingTunnel(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// if the request doesn't include a port and the protocol is HTTP, use port
	// 80 as a default
	if !strings.Contains(r.Host, ":") && r.Header.Get("X-Forwarded-Proto") == "http" {
		host += ":80"
	}

	logger.Printf("dispatching traffic to %q", host)

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	dialer := &net.Dialer{
		Timeout:   20 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}

	errorResponse := &http.Response{
		StatusCode: http.StatusBadGateway,
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		ProtoMinor: 0,
		Header:     make(http.Header),
	}

	dest_conn, err := dialer.Dial("tcp", host)
	if err != nil {
		errorMsg := fmt.Sprintf("error dialing %q: %v", host, err)
		errorResponse.Body = ioutil.NopCloser(bytes.NewBufferString(errorMsg))
		errorResponse.ContentLength = int64(len(errorMsg))
		errorResponse.Write(w)
		return
	}

	err = streamio.DualStream(dest_conn, r.Body, w, dest_conn, &bufferPool)
	if err != nil && !http2error.IsClientDisconnect(err) {
		errorMsg := fmt.Sprintf("error streaming: %v", err)
		errorResponse.ContentLength = int64(len(errorMsg))
		errorResponse.Body = ioutil.NopCloser(bytes.NewBufferString(errorMsg))
		errorResponse.Write(w)
		return
	}
}

func handleOutgoingTunnel(w http.ResponseWriter, r *http.Request) {
	logger := logger.WithFields(log.Fields{
		"handler": "HTTPS",
		"host":    r.Host,
	})

	logger.Printf("proxying request")

	tunnelWriter, tunnelRes, err := tunnel.Create(client, r.Host, *flagConnectorServer, tunnel.RequestHTTPS, tunnel.DispatcherEndpoint)
	if err != nil {
		logger.WithFields(log.Fields{
			"operation": "create tunnel connection",
		}).Errorf("%v", err)
		http.Error(w, fmt.Sprintf("failed to create tunnel to Wormhole connector: %v", err), http.StatusBadGateway)
		return
	}
	defer tunnelRes.Body.Close()

	// HTTP/1: we hijack the connection and send it through the HTTP/2 tunnel
	if r.ProtoMajor == 1 {
		if err := connection.ServeHijack(w, tunnelWriter, tunnelRes.Body, &bufferPool); err != nil {
			logger.WithFields(log.Fields{
				"operation": "hijack HTTP/1 connection",
			}).Errorf("%v", err)
		}
		return
	}

	defer r.Body.Close()

	wFlusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error("ResponseWriter doesn't implement Flusher()")
		http.Error(w, "ResponseWriter doesn't implement Flusher()", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	wFlusher.Flush()

	err = streamio.DualStream(tunnelWriter, r.Body, w, tunnelRes.Body, &bufferPool)
	if err != nil && !http2error.IsClientDisconnect(err) {
		logger.WithFields(log.Fields{
			"operation": "copy proxy streams",
		}).Errorf("%v", err)
		http.Error(w, fmt.Sprintf("failed to copy proxy streams: %v", err), http.StatusBadGateway)
		return
	}
}

func handleOutgoingHTTP(w http.ResponseWriter, r *http.Request) {
	logger := logger.WithFields(log.Fields{
		"handler": "HTTP",
		"host":    r.Host,
	})

	logger.Infof("proxying request")

	tunnelWriter, tunnelRes, err := tunnel.Create(client, r.Host, *flagConnectorServer, tunnel.RequestHTTP, tunnel.DispatcherEndpoint)
	if err != nil {
		logger.WithFields(log.Fields{
			"operation": "create tunnel connection",
		}).Errorf("%v", err)
		http.Error(w, fmt.Sprintf("failed to create tunnel to Wormhole connector: %v", err), http.StatusBadGateway)
		return
	}
	defer tunnelRes.Body.Close()

	header.RemoveHopByHop(r.Header)

	if err := r.Write(tunnelWriter); err != nil {
		logger.WithFields(log.Fields{
			"operation": "write request to tunnel",
		}).Errorf("%v", err)
		http.Error(w, fmt.Sprintf("failed to write request to tunnel: %v", err), http.StatusBadGateway)
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(tunnelRes.Body), r)
	if err != nil {
		logger.WithFields(log.Fields{
			"operation": "read response from proxy stream",
		}).Errorf("%v", err)
		http.Error(w, fmt.Sprintf("failed to read response from proxy stream: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	header.Copy(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	buf := bufferPool.Get().([]byte)
	buf = buf[0:cap(buf)]

	if _, err := io.CopyBuffer(w, resp.Body, buf); err != nil {
		logger.WithFields(log.Fields{
			"operation": "copy stream response from proxy stream",
		}).Errorf("%v", err)
		http.Error(w, fmt.Sprintf("failed to copy response from proxy stream: %v", err), http.StatusBadGateway)
		return
	}
}

func dispatcherHandler(w http.ResponseWriter, r *http.Request) {
	logger := log.WithFields(log.Fields{"component": "dispatcherHandler"})

	if r.Method != http.MethodConnect {
		logger.Errorf(fmt.Sprintf("method %q not implemented", r.Method))
		http.Error(w, fmt.Sprintf("method %q not implemented", r.Method), http.StatusNotImplemented)
		return
	}

	if r.Header.Get("X-Wormhole-Connector") == "true" {
		handleIncomingTunnel(w, r)
	} else {
		handleOutgoingTunnel(w, r)
	}
}

func main() {
	var srv http.Server
	flag.Parse()

	srv.Addr = *flagLocalAddr
	srv.IdleTimeout = 90 * time.Minute
	http2.ConfigureServer(&srv, &http2.Server{})

	makeBuffer := func() interface{} { return make([]byte, 0, 32*1024) }
	bufferPool = sync.Pool{New: makeBuffer}

	tlsConfig, err := tlsutil.GenerateTLSConfig(*flagTrustCAFile, *flagInsecure)
	if err != nil {
		log.Fatal(err)
	}

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client = &http.Client{
		Transport: tr,
	}

	http2.ConfigureTransport(tr)

	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			dispatcherHandler(w, r)
			return
		} else {
			handleOutgoingHTTP(w, r)
		}
	})

	go func() {
		log.Fatal(srv.ListenAndServeTLS("dispatcher.pem", "dispatcher-key.pem"))
	}()

	select {}
}
