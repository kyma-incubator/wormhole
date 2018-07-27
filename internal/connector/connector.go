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
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/hashicorp/serf/serf"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"

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

	// for proxy server
	bufferPool sync.Pool
}

// WormholeConnectorConfig holds all kinds of configurations given by users or
// local config files. It is used when initializing a new wormhole connector.
type WormholeConnectorConfig struct {
	KymaServer      string
	RaftPort        int
	LocalAddr       string
	SerfMemberAddrs string
	SerfPort        int
	Timeout         time.Duration
	DataDir         string
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
			wc.handleProxy(w, r)
			return
		} else {
			wc.handleHTTP(w, r)
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

	tr := &http.Transport{
		// TODO disable this or make it configurable
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout: config.Timeout,
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

	srv.Handler = wc.wormholeHandler(m)

	makeBuffer := func() interface{} { return make([]byte, 0, bufferSize) }
	wc.bufferPool = sync.Pool{New: makeBuffer}

	wc.rpcPort = bindPort

	newRaft, err := NewWormholeRaft(wc, bindAddr, config.RaftPort, config.DataDir)
	if err != nil {
		return nil, err
	}
	wc.WRaft = newRaft

	newSerf, err := NewWormholeSerf(wc, peers, config.SerfPort)
	if err != nil {
		return nil, err
	}
	wc.WSerf = newSerf

	registerHandlers(m, wc)

	return wc, nil
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

func (wc *WormholeConnector) createTunnelConnection(host string, https bool) (io.Writer, *http.Response, error) {
	pr, pw := io.Pipe()
	tunnelReq, err := http.NewRequest(http.MethodConnect, wc.kymaServerAddr, ioutil.NopCloser(pr))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to prepare request: %v", err)
	}

	// set Host in the tunnel request to the host requested by the client
	tunnelReq.Host = host

	if !https {
		// set protocol so the other end of the tunnel knows what port to dial
		tunnelReq.Header.Set("X-Forwarded-Proto", "http")
	}

	// establish tunnel, if a tunnel is already established, the same HTTP/2
	// connection will be used
	tunnelRes, err := wc.client.Do(tunnelReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to do request: %v", err)
	}

	return pw, tunnelRes, nil
}

func (wc *WormholeConnector) handleHTTP(w http.ResponseWriter, r *http.Request) {
	tunnelWriter, tunnelRes, err := wc.createTunnelConnection(r.Host, false)
	if err != nil {
		log.Errorf("failed to create tunnel to Kyma server: %v", err)
		http.Error(w, fmt.Sprintf("failed to create tunnel to Kyma server: %v", err), http.StatusBadGateway)
		return
	}
	defer tunnelRes.Body.Close()

	removeHopByHop(r.Header)

	if err := r.Write(tunnelWriter); err != nil {
		log.Errorf("failed to write request to proxy stream: %v", err)
		http.Error(w, fmt.Sprintf("failed to write request to proxy stream: %v", err), http.StatusBadGateway)
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(tunnelRes.Body), r)
	if err != nil {
		log.Errorf("failed to read response from proxy stream: %v", err)
		http.Error(w, fmt.Sprintf("failed to read response from proxy stream: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	buf := wc.bufferPool.Get().([]byte)
	buf = buf[0:cap(buf)]

	if _, err := io.CopyBuffer(w, resp.Body, buf); err != nil {
		log.Errorf("failed to copy response from proxy stream: %v", err)
		http.Error(w, fmt.Sprintf("failed to copy response from proxy stream: %v", err), http.StatusBadGateway)
		return
	}
}

func (wc *WormholeConnector) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		log.Errorf("method %q not implemented", r.Method)
		http.Error(w, fmt.Sprintf("method %q not implemented", r.Method), http.StatusNotImplemented)
		return
	}

	tunnelWriter, tunnelRes, err := wc.createTunnelConnection(r.Host, true)
	if err != nil {
		log.Errorf("failed to create tunnel to Kyma server: %v", err)
		http.Error(w, fmt.Sprintf("failed to create tunnel to Kyma server: %v", err), http.StatusBadGateway)
		return
	}
	defer tunnelRes.Body.Close()

	// HTTP/1: we hijack the connection and send it through the HTTP/2 tunnel
	if r.ProtoMajor == 1 {
		if err := wc.serveHijack(w, tunnelWriter, tunnelRes.Body); err != nil {
			log.Errorf("failed to hijack HTTP/1 connection: %v", err)
		}
		return
	}

	defer r.Body.Close()

	wFlusher, ok := w.(http.Flusher)
	if !ok {
		log.Error("ResponseWriter doesn't implement Flusher()")
		http.Error(w, "ResponseWriter doesn't implement Flusher()", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	wFlusher.Flush()

	if err := wc.dualStream(tunnelWriter, r.Body, w, tunnelRes.Body); err != nil {
		log.Errorf("failed to copy proxy streams: %v", err)
		http.Error(w, fmt.Sprintf("failed to copy proxy streams: %v", err), http.StatusBadGateway)
		return
	}
}

// dualStream copies data r1->w1 and r2->w2, flushes as needed, and returns
// when both streams are done.
func (wc *WormholeConnector) dualStream(w1 io.Writer, r1 io.Reader, w2 io.Writer, r2 io.Reader) error {
	errChan := make(chan error)

	stream := func(w io.Writer, r io.Reader) {
		buf := wc.bufferPool.Get().([]byte)
		buf = buf[0:cap(buf)]
		_, _err := flushingIoCopy(w, r, buf)
		errChan <- _err
	}

	go stream(w1, r1)
	go stream(w2, r2)
	err1 := <-errChan
	err2 := <-errChan
	if err1 != nil {
		return err1
	}
	return err2
}

// serveHijack hijacks the connection from ResponseWriter, writes the response
// and proxies data between targetConn and hijacked connection.
func (wc *WormholeConnector) serveHijack(w http.ResponseWriter, writ io.Writer, read io.Reader) error {
	_, ok := w.(http.CloseNotifier)
	if !ok {
		return errors.New("ResponseWriter does not implement CloseNotifier")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("ResponseWriter does not implement Hijacker")
	}
	clientConn, bufReader, err := hijacker.Hijack()
	if err != nil {
		return fmt.Errorf("failed to hijack: %v", err)
	}
	defer clientConn.Close()

	// bufReader may contain unprocessed buffered data from the client.
	if bufReader != nil {
		if n := bufReader.Reader.Buffered(); n > 0 {
			rbuf, err := bufReader.Reader.Peek(n)
			if err != nil {
				return err
			}
			writ.Write(rbuf)
		}
	}

	// Since we hijacked the connection, we can't write and flush headers via w.
	// Let's handcraft the response and send it manually.
	res := &http.Response{StatusCode: http.StatusOK,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	res.Header.Set("Server", "wormhole-connector")

	err = res.Write(clientConn)
	if err != nil {
		return fmt.Errorf("failed to send response to client: %v", err)
	}

	return wc.dualStream(writ, clientConn, clientConn, read)
}

func registerHandlers(mux *mux.Router, wc *WormholeConnector) {
	// redirect every request to the raft leader
	mux.PathPrefix("/").HandlerFunc(wc.handleLeaderRedirect)
}

// ListenAndServeTLS spawns a goroutine that runs http.ListenAndServeTLS.
func (wc *WormholeConnector) ListenAndServeTLS(cert, key string) {
	go func() {
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
