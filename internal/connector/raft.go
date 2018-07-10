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
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hashicorp/raft"
	"github.com/kinvolk/wormhole-connector/lib"
)

type WormholeRaft struct {
	wc *WormholeConnector

	raftAddr string
	raftPort int
	rf       *raft.Raft
}

func getNewRaft(raftAddr string, raftPort int, id, dataDir string) (*raft.Raft, error) {
	raftDataBase := filepath.Join(dataDir, "raft")
	if err := os.MkdirAll(raftDataBase, os.FileMode(0755)); err != nil {
		return nil, err
	}

	raftDataDir := filepath.Join(raftDataBase, id)
	if err := os.RemoveAll(raftDataDir + "/"); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(raftDataDir, 0777); err != nil {
		return nil, err
	}

	rf, err := lib.GetNewRaft(raftDataDir, raftAddr, raftPort)
	if err != nil {
		return nil, err
	}

	return rf, nil
}

func NewWormholeRaft(pWc *WormholeConnector, lAddr string, rPort int, dataDir string) *WormholeRaft {
	rAddr := lAddr + ":" + strconv.Itoa(rPort)
	id := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s:%d", lAddr, rPort))))

	newRf, err := getNewRaft(rAddr, rPort, id, dataDir)
	if err != nil {
		return nil
	}

	return &WormholeRaft{
		wc: pWc,

		raftAddr: rAddr,
		raftPort: rPort,
		rf:       newRf,
	}
}

func (wr *WormholeRaft) BootstrapRaft(peeraddrs []string) error {
	bootstrapConfig := raft.Configuration{
		Servers: []raft.Server{
			{
				Suffrage: raft.Voter,
				ID:       raft.ServerID(wr.raftAddr),
				Address:  raft.ServerAddress(wr.raftAddr),
			},
		},
	}

	// Add known serf peer addresses to bootstrap
	for _, addr := range peeraddrs {
		if addr == wr.raftAddr {
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

func (wr *WormholeRaft) IsLeader() bool {
	if wr.rf.Leader() == "" {
		return false
	} else if wr.rf.State() == raft.Leader {
		return true
	}
	return false
}

func (wr *WormholeRaft) AddVoter(changedPeer string) error {
	rf := wr.rf
	indexFuture := rf.AddVoter(raft.ServerID(changedPeer), raft.ServerAddress(changedPeer), 0, 0)
	if err := indexFuture.Error(); err != nil {
		return fmt.Errorf("error adding voter: %s", err)
	}
	return nil
}

func (wr *WormholeRaft) RemoveServer(changedPeer string) error {
	rf := wr.rf
	indexFuture := rf.RemoveServer(raft.ServerID(changedPeer), 0, 0)
	if err := indexFuture.Error(); err != nil {
		return fmt.Errorf("error removing server: %s", err)
	}
	return nil
}
