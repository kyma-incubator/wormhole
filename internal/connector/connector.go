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
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/hashicorp/serf/serf"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"

	"github.com/kinvolk/wormhole-connector/lib"
)

type WormholeConnector struct {
	localAddr string
	raftAddr  string
	raftPort  int

	serfDB     *lib.SerfDB
	serfEvents chan serf.Event
	serfPeers  []lib.SerfPeer
	serfPort   int

	server  http.Server
	workDir string
}

type WormholeConnectorConfig struct {
	KymaServer      string
	RaftPort        int
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
		localAddr: localAddr,
		raftPort:  config.RaftPort,
		serfPeers: peers,
		serfPort:  config.SerfPort,
		server:    srv,
		workDir:   config.WorkDir,
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

func registerHandlers(mux *mux.Router, wc *WormholeConnector) {
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
