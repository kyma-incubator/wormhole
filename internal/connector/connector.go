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

package connector

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/hashicorp/serf/serf"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"

	"github.com/kinvolk/wormhole-connector/internal/connection"
	"github.com/kinvolk/wormhole-connector/internal/header"
	"github.com/kinvolk/wormhole-connector/internal/http2error"
	"github.com/kinvolk/wormhole-connector/internal/streamio"
	"github.com/kinvolk/wormhole-connector/internal/tlsutil"
	"github.com/kinvolk/wormhole-connector/internal/tunnel"
	"github.com/kinvolk/wormhole-connector/lib"
)

const (
	bufferSize = 32 * 1024
)

// WormholeConnector is a core structure for handling communication of a
// wormhole connector, such as HTTP2 connections, Serf and Raft.
type WormholeConnector struct {
	localAddr string

	WRaft *WormholeRaft
	WSerf *WormholeSerf

	server         *http.Server
	client         *http.Client
	rpcPort        string
	dataDir        string
	kymaServerAddr string

	logger *log.Entry

	// for proxy server
	bufferPool sync.Pool
}

// WormholeConnectorConfig holds all kinds of configurations given by users or
// local config files. It is used when initializing a new wormhole connector.
type WormholeConnectorConfig struct {
	KymaServer            string
	KymaReverseTunnelPort int
	RaftPort              int
	LocalAddr             string
	SerfMemberAddrs       string
	SerfPort              int
	Timeout               time.Duration
	DataDir               string
	TrustCAFile           string
	Insecure              bool
}

// splitLocalAddr takes a localAddr[:port] address and returns its address and
// port components, if no port is specified, it is assumed to be defaultPort
func splitLocalAddr(localAddr, defaultPort string) (string, string, error) {
	if strings.Contains(localAddr, ":") {
		return net.SplitHostPort(localAddr)
	}

	return localAddr, defaultPort, nil
}

func (wc *WormholeConnector) wormholeHandler(m *mux.Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			wc.handleOutgoingTunnel(w, r)
			return
		} else {
			wc.handleOutgoingHTTP(w, r)
		}
	})
}

// NewWormholeConnector returns a new wormhole connector, which holds e.g.,
// local IP address, http server, location of data directory, and pointers to
// WormholeSerf & WormholeRaft structures. This function implicitly initializes
// both WormholeSerf and WormholeRaft, and registers necessary HTTP handlers
// as well.
func NewWormholeConnector(config WormholeConnectorConfig) (*WormholeConnector, error) {
	var srv http.Server
	var client http.Client

	tlsConfig, err := tlsutil.GenerateTLSConfig(config.TrustCAFile, config.Insecure)
	if err != nil {
		return nil, err
	}

	tr := &http.Transport{
		IdleConnTimeout: config.Timeout,
		TLSClientConfig: tlsConfig,
	}
	client = http.Client{Transport: tr}

	http2.ConfigureTransport(tr)

	bindAddr, bindPort, err := splitLocalAddr(config.LocalAddr, "8080")
	if err != nil {
		return nil, fmt.Errorf("error splitting local-addr: %v", err)
	}

	srv.Addr = fmt.Sprintf("%s:%s", bindAddr, bindPort)
	http2.ConfigureServer(&srv, &http2.Server{})

	m := mux.NewRouter()

	var peerAddrs []string
	var peers []lib.SerfPeer

	if config.SerfMemberAddrs != "" {
		peerAddrs = strings.Split(config.SerfMemberAddrs, ",")
	}

	for _, addr := range peerAddrs {
		peers = append(peers, lib.SerfPeer{
			PeerName: addr, // it should be actually hostname
			Address:  addr,
		})
	}

	wc := &WormholeConnector{
		localAddr:      config.LocalAddr,
		server:         &srv,
		client:         &client,
		dataDir:        config.DataDir,
		kymaServerAddr: config.KymaServer,
	}

	wc.logger = log.WithFields(log.Fields{"component": "connector"})

	srv.Handler = wc.addRequestID(wc.wormholeHandler(m))

	makeBuffer := func() interface{} { return make([]byte, 0, bufferSize) }
	wc.bufferPool = sync.Pool{New: makeBuffer}

	wc.rpcPort = bindPort

	newRaft, err := NewWormholeRaft(wc, bindAddr, config.RaftPort, config.DataDir)
	if err != nil {
		return nil, err
	}
	wc.WRaft = newRaft

	newSerf, err := NewWormholeSerf(wc, peers, bindAddr, config.SerfPort)
	if err != nil {
		return nil, err
	}
	wc.WSerf = newSerf

	registerHandlers(m, wc)

	go wc.reverseTunnel(config.KymaReverseTunnelPort, tlsConfig)

	return wc, nil
}

func (wc *WormholeConnector) reverseTunnel(port int, tlsConfig *tls.Config) error {
	for {
		time.Sleep(1 * time.Second)

		u, err := url.Parse(wc.kymaServerAddr)
		if err != nil {
			wc.logger.Fatalf("error parsing Kyma server address: %v", err)
		}

		host := strings.Split(u.Host, ":")[0]

		dialer := &net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}

		c, err := tls.DialWithDialer(
			dialer,
			"tcp",
			fmt.Sprintf("%s:%d", host, port),
			tlsConfig,
		)
		if err != nil {
			wc.logger.Debugf("failed to connect to Wormhole Dispatcher: %v", err)
			wc.logger.Debug("retrying...")
			continue
		}

		if err := c.Handshake(); err != nil {
			wc.logger.Debugf("failed do TLS handshake to Wormhole Dispatcher: %v", err)
			wc.logger.Debug("retrying...")
			continue
		}

		log.Infof("connected to Wormhole Dispatcher")
		h2s := &http2.Server{}
		h2s.ServeConn(c, &http2.ServeConnOpts{Handler: http.HandlerFunc(wc.handleIncomingTunnel)})
		log.Infof("disconnected from Wormhole Dispatcher")
	}
}

func (wc *WormholeConnector) addRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), "logger", wc.logger.WithFields(log.Fields{
			"request_id": uuid.NewV4(),
		}))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (wc *WormholeConnector) handleLeaderRedirect(w http.ResponseWriter, r *http.Request) {
	wr := wc.WRaft
	if wr.IsLeader() {
		// This node is a leader, handle the real request
		wr.handleEvents(w, r)
		return
	}

	// It means this node is not a leader, but a follower.
	// Redirect every request to the leader.
	// wr.rf.Leader() returns a string in format of LEADER_IP_ADDRESS:LEADER_RAFT_PORT,
	// e.g. 172.17.0.2:1112, so we need to split it up to get only the first
	// part, to append rpcPort, e.g. 8080.
	leaderAddress := string(wr.rf.Leader())

	if leaderAddress == "" {
		// there's no leader, so let's return a special error message
		http.Error(w, "unable to get leader address, as there's no leader", http.StatusInternalServerError)
		return
	}

	leaderHost, _, err := net.SplitHostPort(leaderAddress)
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to parse leader address: %v", err), http.StatusInternalServerError)
		return
	}

	leaderURL := fmt.Sprintf("https://%v:%v", leaderHost, wc.rpcPort)
	http.Redirect(w, r, leaderURL, http.StatusTemporaryRedirect)
}

func (wc *WormholeConnector) handleOutgoingHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctxLogger := getLogger(ctx)
	logger := ctxLogger.WithFields(log.Fields{
		"handler": "outgoing HTTP",
		"host":    r.Host,
	})

	logger.Infof("proxying request")

	tunnelWriter, tunnelRes, err := tunnel.Create(wc.client, r.Host, wc.kymaServerAddr, tunnel.RequestHTTP, tunnel.ConnectorEndpoint)
	if err != nil {
		logger.WithFields(log.Fields{
			"operation": "create tunnel connection",
		}).Errorf("%v", err)
		http.Error(w, fmt.Sprintf("failed to create tunnel to Kyma server: %v", err), http.StatusBadGateway)
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

	buf := wc.bufferPool.Get().([]byte)
	buf = buf[0:cap(buf)]

	if _, err := io.CopyBuffer(w, resp.Body, buf); err != nil {
		logger.WithFields(log.Fields{
			"operation": "copy stream response from proxy stream",
		}).Errorf("%v", err)
		http.Error(w, fmt.Sprintf("failed to copy response from proxy stream: %v", err), http.StatusBadGateway)
		return
	}
}

func (wc *WormholeConnector) handleOutgoingTunnel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctxLogger := getLogger(ctx)

	logger := ctxLogger.WithFields(log.Fields{
		"handler": "outgoing HTTPS",
		"host":    r.Host,
	})

	if r.Method != http.MethodConnect {
		logger.Errorf("method %q not implemented", r.Method)
		http.Error(w, fmt.Sprintf("method %q not implemented", r.Method), http.StatusNotImplemented)
		return
	}

	if r.Header.Get("X-Wormhole-Dispatcher") == "true" {
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

		err = streamio.DualStream(dest_conn, r.Body, w, dest_conn, &wc.bufferPool)
		if err != nil && !http2error.IsClientDisconnect(err) {
			errorMsg := fmt.Sprintf("error streaming: %v", err)
			errorResponse.ContentLength = int64(len(errorMsg))
			errorResponse.Body = ioutil.NopCloser(bytes.NewBufferString(errorMsg))
			errorResponse.Write(w)
			return
		}
	} else {
		logger.Infof("proxying request")

		tunnelWriter, tunnelRes, err := tunnel.Create(wc.client, r.Host, wc.kymaServerAddr, tunnel.RequestHTTPS, tunnel.ConnectorEndpoint)
		if err != nil {
			logger.WithFields(log.Fields{
				"operation": "create tunnel connection",
			}).Errorf("%v", err)
			http.Error(w, fmt.Sprintf("failed to create tunnel to Kyma server: %v", err), http.StatusBadGateway)
			return
		}
		defer tunnelRes.Body.Close()

		// HTTP/1: we hijack the connection and send it through the HTTP/2 tunnel
		if r.ProtoMajor == 1 {
			if err := connection.ServeHijack(w, tunnelWriter, tunnelRes.Body, &wc.bufferPool); err != nil {
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

		err = streamio.DualStream(tunnelWriter, r.Body, w, tunnelRes.Body, &wc.bufferPool)
		if err != nil && !http2error.IsClientDisconnect(err) {
			logger.WithFields(log.Fields{
				"operation": "copy proxy streams",
			}).Errorf("%v", err)
			http.Error(w, fmt.Sprintf("failed to copy proxy streams: %v", err), http.StatusBadGateway)
			return
		}
	}
}

func (wc *WormholeConnector) handleIncomingTunnel(w http.ResponseWriter, r *http.Request) {
	logger := log.WithFields(log.Fields{
		"handler": "incoming",
		"host":    r.Host,
	})

	// We need to do some dummy request to avoid running into the HTTP/2
	// preface timeout, which is not configurable in the server
	// (https://github.com/golang/net/blob/f9ce57c11/http2/server.go#L919)
	if r.Method == http.MethodGet {
		w.Write([]byte("PONG"))
		return
	}

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

	err = streamio.DualStream(dest_conn, r.Body, w, dest_conn, &wc.bufferPool)
	if err != nil && !http2error.IsClientDisconnect(err) {
		errorMsg := fmt.Sprintf("error streaming: %v", err)
		errorResponse.ContentLength = int64(len(errorMsg))
		errorResponse.Body = ioutil.NopCloser(bytes.NewBufferString(errorMsg))
		errorResponse.Write(w)
		return
	}
}

func registerHandlers(mux *mux.Router, wc *WormholeConnector) {
	// redirect every request to the raft leader
	mux.PathPrefix("/").HandlerFunc(wc.handleLeaderRedirect)
}

// ListenAndServeTLS spawns a goroutine that runs http.ListenAndServeTLS.
func (wc *WormholeConnector) ListenAndServeTLS(cert, key string) {
	go func() {
		wc.logger.Infof("listening for client requests on %s", wc.server.Addr)
		if err := wc.server.ListenAndServeTLS(cert, key); err != nil {
			if err != http.ErrServerClosed {
				log.Fatal(err)
			}
		}
	}()
}

// Shutdown shuts down the http server of WormholeConnector, and it also
// destroys its Serf object.
func (wc *WormholeConnector) Shutdown(ctx context.Context) {
	wc.server.Shutdown(ctx)
	wc.WSerf.Shutdown()
	wc.WRaft.Shutdown()
}

func getLogger(ctx context.Context) *log.Entry {
	logger, ok := ctx.Value("logger").(*log.Entry)
	if !ok {
		return nil
	}
	return logger
}

// SetupSerfRaft initializes a Serf cluster being joined by the given peers,
// and it bootstraps a Raft cluster with the peers.
func (wc *WormholeConnector) SetupSerfRaft() error {
	if wc.WSerf == nil || wc.WRaft == nil {
		return fmt.Errorf("unable to set up serf and raft. WSerf == %v, WRaft == %v", wc.WSerf, wc.WRaft)
	}

	if err := wc.WSerf.SetupSerf(); err != nil {
		return err
	}

	if err := wc.WRaft.BootstrapRaft(wc.WSerf.GetPeerAddrs()); err != nil {
		return err
	}

	return nil
}

// ProbeSerfRaft goes into a loop where Raft status becomes verified, and
// serf events are handled correspondently.
func (wc *WormholeConnector) ProbeSerfRaft(sigchan chan os.Signal) error {
	ticker := time.NewTicker(3 * time.Second)

	for {
		select {
		case <-sigchan:
			// catch SIGTERM to quit immediately
			return nil
		case <-ticker.C:
			if err := wc.WRaft.VerifyRaft(); err != nil {
				return fmt.Errorf("unable to probe raft peers: %v", err)
			}

		case ev := <-wc.WSerf.serfEvents:
			if err := wc.handleSerfEvents(ev); err != nil {
				return fmt.Errorf("unable to probe serf peers: %v", err)
			}
		}
	}
}

func (wc *WormholeConnector) handleSerfEvents(ev serf.Event) error {
	wRaft := wc.WRaft
	memberEvent, ok := ev.(serf.MemberEvent)
	if !ok {
		return nil
	}

	for _, member := range memberEvent.Members {
		// TODO: remove restriction of the raft port having to be serf port + 1
		changedPeer := member.Addr.String() + ":" + strconv.Itoa(int(member.Port+1))
		if memberEvent.EventType() == serf.EventMemberJoin {
			if !wRaft.IsLeader() {
				continue
			}
			if err := wRaft.AddVoter(changedPeer); err != nil {
				return err
			}
		} else if lib.IsMemberEventFailed(memberEvent) {
			if !wRaft.IsLeader() {
				continue
			}
			if err := wRaft.RemoveServer(changedPeer); err != nil {
				return err
			}
		}
	}

	return nil
}
