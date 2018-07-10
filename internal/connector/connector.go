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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"

	"github.com/hashicorp/serf/serf"
	"github.com/kinvolk/wormhole-connector/lib"
)

type WormholeConnector struct {
	localAddr string

	WRaft *WormholeRaft
	WSerf *WormholeSerf

	server  http.Server
	rpcPort string
	dataDir string
}

type WormholeConnectorConfig struct {
	KymaServer      string
	RaftPort        int
	LocalAddr       string
	SerfMemberAddrs string
	SerfPort        int
	Timeout         time.Duration
	DataDir         string
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
		server:    srv,
		dataDir:   config.DataDir,
	}

	wc.WRaft = NewWormholeRaft(wc, config.LocalAddr, config.RaftPort, config.DataDir)
	wc.WSerf = NewWormholeSerf(wc, peers, config.SerfPort)

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
	wr := wc.WRaft
	if wr.IsLeader() {
		// This node is a leader, so there's nothing to do.
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
	w.WSerf.Shutdown()
}

func getLogger(ctx context.Context) *log.Entry {
	logger, ok := ctx.Value("logger").(*log.Entry)
	if !ok {
		return nil
	}
	return logger
}

func (wc *WormholeConnector) SetupSerfRaft() error {
	if err := wc.WSerf.SetupSerf(); err != nil {
		return err
	}

	if err := wc.WRaft.BootstrapRaft(wc.WSerf.GetPeerAddrs()); err != nil {
		return err
	}

	return nil
}

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
			if err := wc.HandleSerfEvents(ev); err != nil {
				return fmt.Errorf("unable to probe serf peers: %v", err)
			}
		}
	}
}

func (wc *WormholeConnector) HandleSerfEvents(ev serf.Event) error {
	wraft := wc.WRaft
	memberEvent, ok := ev.(serf.MemberEvent)
	if !ok {
		return nil
	}

	for _, member := range memberEvent.Members {
		changedPeer := member.Addr.String() + ":" + strconv.Itoa(int(member.Port+1))
		if memberEvent.EventType() == serf.EventMemberJoin {
			if !wraft.IsLeader() {
				continue
			}
			if err := wraft.AddVoter(changedPeer); err != nil {
				return err
			}
		} else if lib.IsMemberEventFailed(memberEvent) {
			if !wraft.IsLeader() {
				continue
			}
			if err := wraft.RemoveServer(changedPeer); err != nil {
				return err
			}
		}
	}

	return nil
}
