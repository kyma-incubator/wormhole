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
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hashicorp/raft"
	log "github.com/sirupsen/logrus"

	"github.com/kinvolk/wormhole-connector/lib"
)

var (
	defaultEventTimeout = 10 * time.Second
)

// WormholeRaft holds runtime information for Raft, such as IP address,
// TCP port, and a pointer to the underlying Raft structure.
type WormholeRaft struct {
	wc *WormholeConnector

	events    *lib.EventsFSM
	logWriter *os.File

	raftListenPeerAddr string
	raftListenPeerPort int
	rf                 *raft.Raft
}

func getNewRaft(raftListenPeerAddr string, raftListenPeerPort int, fsm raft.FSM, id, dataDir string) (*raft.Raft, *os.File, error) {
	raftDataBase := filepath.Join(dataDir, "raft")
	if err := os.MkdirAll(raftDataBase, os.FileMode(0755)); err != nil {
		return nil, nil, err
	}

	raftDataDir := filepath.Join(raftDataBase, id)
	if err := os.RemoveAll(raftDataDir + "/"); err != nil {
		return nil, nil, err
	}

	if err := os.MkdirAll(raftDataDir, 0777); err != nil {
		return nil, nil, err
	}

	logFile := filepath.Join(raftDataDir, "raft.log")
	logWriter, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	rf, err := lib.GetNewRaft(logWriter, raftDataDir, raftListenPeerAddr, raftListenPeerPort, fsm)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to open file %s: %v", logFile, err)
	}

	return rf, logWriter, nil
}

// NewWormholeRaft returns a new wormhole raft object, which holds e.g.,
// TCP transport information and the underlying Raft structure.
func NewWormholeRaft(pWc *WormholeConnector, lAddr string, rPort int, dataDir string) (*WormholeRaft, error) {
	rAddr := lAddr + ":" + strconv.Itoa(rPort)
	id := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s:%d", lAddr, rPort))))

	events := lib.NewEventsFSM()

	newRf, logWriter, err := getNewRaft(rAddr, rPort, events, id, dataDir)
	if err != nil {
		return nil, err
	}

	return &WormholeRaft{
		wc: pWc,

		events:             events,
		logWriter:          logWriter,
		raftListenPeerAddr: rAddr,
		raftListenPeerPort: rPort,
		rf:                 newRf,
	}, nil
}

func (wr *WormholeRaft) handleEvents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		event := wr.events.TopEvent()
		w.Write([]byte(event))
	case "POST":
		event := r.FormValue("event")
		wr.EnqueueEvent(event)
	case "DELETE":
		wr.DiscardTopEvent()
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

// BootstrapRaft initializes a Raft cluster and bootstraps it with the list of
// given peers.
func (wr *WormholeRaft) BootstrapRaft(peerAddrs []string) error {
	bootstrapConfig := raft.Configuration{
		Servers: []raft.Server{
			{
				Suffrage: raft.Voter,
				ID:       raft.ServerID(wr.raftListenPeerAddr),
				Address:  raft.ServerAddress(wr.raftListenPeerAddr),
			},
		},
	}

	// Add known serf peer addresses to bootstrap
	for _, addr := range peerAddrs {
		if addr == wr.raftListenPeerAddr {
			continue
		}

		bootstrapConfig.Servers = append(bootstrapConfig.Servers, raft.Server{
			Suffrage: raft.Voter,
			ID:       raft.ServerID(addr),
			Address:  raft.ServerAddress(addr),
		})
	}

	return wr.rf.BootstrapCluster(bootstrapConfig).Error()
}

// Shutdown destroys everything for Raft before shutting down the wormhole
// connector.
func (wr *WormholeRaft) Shutdown() {
	wr.logWriter.Close()
}

// VerifyRaft checks for the status of the current raft node, and prints it out.
func (wr *WormholeRaft) VerifyRaft() error {
	if wr.IsLeader() {
		fmt.Println("Node is leader")
	} else {
		fmt.Println("Node is a follower")
	}

	cfuture := wr.rf.GetConfiguration()
	if err := cfuture.Error(); err != nil {
		return fmt.Errorf("error getting config: %s", err)
	}

	configuration := cfuture.Configuration()
	for _, server := range configuration.Servers {
		fmt.Println(server.Address)
	}

	return nil
}

// IsLeader returns true if the current Raft node is a valid leader.
// Otherwise it returns false.
func (wr *WormholeRaft) IsLeader() bool {
	return wr.rf.State() == raft.Leader
}

// AddVoter is a simple wrapper around Raft.AddVoter(), which adds the
// given server to the cluster as a staging server, making a voter.
func (wr *WormholeRaft) AddVoter(changedPeer string) error {
	rf := wr.rf
	indexFuture := rf.AddVoter(raft.ServerID(changedPeer), raft.ServerAddress(changedPeer), 0, 0)
	if err := indexFuture.Error(); err != nil {
		return fmt.Errorf("error adding voter: %s", err)
	}
	return nil
}

// RemoveServer is a simple wrapper around Raft.RemoveServer(), which removes
// the given server from the cluster.
func (wr *WormholeRaft) RemoveServer(changedPeer string) error {
	rf := wr.rf
	indexFuture := rf.RemoveServer(raft.ServerID(changedPeer), 0, 0)
	if err := indexFuture.Error(); err != nil {
		return fmt.Errorf("error removing server: %s", err)
	}
	return nil
}

func (wr *WormholeRaft) apply(a *lib.Action, timeout time.Duration) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}

	f := wr.rf.Apply(data, timeout)
	return f.Error()
}

func (wr *WormholeRaft) EnqueueEvent(ev string) error {
	if wr == nil {
		wr.events.EnqueueEvent(ev)
		return nil
	}

	if !wr.IsLeader() {
		log.Info("is not leader, skip")
		return nil
	}

	var a = lib.Action{
		Cmd:   lib.EnqueueCmd,
		Event: ev,
	}

	return wr.apply(&a, defaultEventTimeout)
}

func (wr *WormholeRaft) DiscardTopEvent() error {
	if wr == nil {
		wr.events.DiscardTopEvent()
		return nil
	}

	if !wr.IsLeader() {
		log.Info("is not leader, skip")
		return nil
	}

	var a = lib.Action{
		Cmd:   lib.DiscardCmd,
		Event: "",
	}

	return wr.apply(&a, defaultEventTimeout)
}
