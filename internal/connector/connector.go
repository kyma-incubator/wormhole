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
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/serf/serf"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"

	"github.com/kinvolk/wormhole-connector/lib"
)

type WormholeConnector struct {
	localAddr string

	raftAddr string
	raftPort int
	rf       *raft.Raft

	serfDB     *lib.SerfDB
	serfEvents chan serf.Event
	serfPeers  []lib.SerfPeer
	serfPort   int

	server  http.Server
	rpcPort string
	workDir string
}

type WormholeConnectorConfig struct {
	KymaServer      string
	RaftPort        int
	LocalAddr       string
	SerfMemberAddrs string
	SerfPort        int
	Timeout         time.Duration
	WorkDir         string
}

func NewWormholeConnector(config WormholeConnectorConfig) *WormholeConnector {
	var srv http.Server

	srv.Addr = config.KymaServer
	srv.IdleTimeout = config.Timeout
	http2.ConfigureServer(&srv, &http2.Server{})

	m := mux.NewRouter()
	srv.Handler = addLogger(m)

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
		localAddr: config.LocalAddr,
		raftPort:  config.RaftPort,
		serfPeers: peers,
		serfPort:  config.SerfPort,
		server:    srv,
		workDir:   config.WorkDir,
	}

	// Split kymaServer in a format of host:port into parts, and store the 2nd
	// part into rpcPort. If it's not possible, fall back to 8080.
	if config.KymaServer != "" && strings.Contains(config.KymaServer, ":") {
		wc.rpcPort = strings.Split(config.KymaServer, ":")[1]
	} else {
		wc.rpcPort = "8080"
	}

	registerHandlers(m, wc)

	return wc
}

func addLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), "logger", log.WithFields(log.Fields{
			"request_id": uuid.NewV4()}))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (wc *WormholeConnector) handleLeaderRedirect(w http.ResponseWriter, r *http.Request) {
	if wc.rf.State() == raft.Leader {
		// This node is a leader, so there's nothing to do.
		return
	}

	// It means this node is not a leader, but a follower.
	// Redirect every request to the leader.
	// wc.rf.Leader() returns a string in format of LEADER_IP_ADDRESS:LEADER_RAFT_PORT,
	// e.g. 172.17.0.2:1112, so we need to split it up to get only the first
	// part, to append rpcPort, e.g. 8080.
	leaderHost, _, err := net.SplitHostPort(string(wc.rf.Leader()))
	if err != nil {
		http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
		return
	}

	leaderURL := fmt.Sprintf("https://%v:%v", leaderHost, wc.rpcPort)
	http.Redirect(w, r, leaderURL, http.StatusTemporaryRedirect)
}

func registerHandlers(mux *mux.Router, wc *WormholeConnector) {
	// redirect every request to the raft leader
	mux.PathPrefix("/").HandlerFunc(wc.handleLeaderRedirect)
}

func (w *WormholeConnector) ListenAndServeTLS(cert, key string) {
	go func() {
		if err := w.server.ListenAndServeTLS(cert, key); err != nil {
			if err != http.ErrServerClosed {
				log.Fatal(err)
			}
		}
	}()
}

func (w *WormholeConnector) Shutdown(ctx context.Context) {
	w.server.Shutdown(ctx)

	defer func() {
		if err := w.serfDB.BoltDB.Close(); err != nil {
			fmt.Printf("cannot close DB\n")
		}
	}()
}

func getLogger(ctx context.Context) *log.Entry {
	logger, ok := ctx.Value("logger").(*log.Entry)
	if !ok {
		return nil
	}
	return logger
}
