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
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"

	"github.com/kinvolk/wormhole-connector/internal/http2error"
	"github.com/kinvolk/wormhole-connector/internal/streamio"
)

var bufferPool sync.Pool

var (
	httpsAddr = flag.String("https_addr", "localhost:4430", "TLS address to listen on ('host:port' or ':port'). Required.")
)

func dispatcherHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		log.Printf(fmt.Sprintf("method %q not implemented", r.Method))
		http.Error(w, fmt.Sprintf("method %q not implemented", r.Method), http.StatusNotImplemented)
		return
	}

	host := r.Host
	// if the request doesn't include a port and the protocol is HTTP, use port
	// 80 as a default
	if !strings.Contains(r.Host, ":") && r.Header.Get("X-Forwarded-Proto") == "http" {
		host += ":80"
	}

	log.Printf("dispatching traffic to %q", host)

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

func main() {
	var srv http.Server
	flag.Parse()

	srv.Addr = *httpsAddr
	srv.IdleTimeout = 90 * time.Minute
	http2.ConfigureServer(&srv, &http2.Server{})

	makeBuffer := func() interface{} { return make([]byte, 0, 32*1024) }
	bufferPool = sync.Pool{New: makeBuffer}

	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			dispatcherHandler(w, r)
			return
		}
		http.Error(w, fmt.Sprintf("method %q not implemented", r.Method), http.StatusNotImplemented)
	})

	go func() {
		log.Fatal(srv.ListenAndServeTLS("dispatcher.pem", "dispatcher-key.pem"))
	}()

	select {}
}
