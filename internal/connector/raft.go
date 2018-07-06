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
	"time"

	"github.com/hashicorp/raft"
	"github.com/kinvolk/wormhole-connector/lib"
)

func (wc *WormholeConnector) getNewRaft() (*raft.Raft, error) {
	id := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s:%d", wc.localAddr, wc.raftPort))))

	raftDataBase := filepath.Join(wc.workDir, "tmp/raft")
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

	wc.raftAddr = wc.localAddr + ":" + strconv.Itoa(wc.raftPort)
	rf, err := lib.GetNewRaft(raftDataDir, wc.raftAddr, wc.raftPort)
	if err != nil {
		return nil, err
	}

	return rf, nil
}

func (wc *WormholeConnector) bootstrapRaft(rf *raft.Raft) error {
	bootstrapConfig := raft.Configuration{
		Servers: []raft.Server{
			{
				Suffrage: raft.Voter,
				ID:       raft.ServerID(wc.raftAddr),
				Address:  raft.ServerAddress(wc.raftAddr),
			},
		},
	}

	// Add known serf peers to bootstrap
	for _, node := range wc.serfPeers {
		nodeAddr := node.Address
		if nodeAddr == wc.raftAddr {
			continue
		}

		bootstrapConfig.Servers = append(bootstrapConfig.Servers, raft.Server{
			Suffrage: raft.Voter,
			ID:       raft.ServerID(nodeAddr),
			Address:  raft.ServerAddress(nodeAddr),
		})
	}

	return rf.BootstrapCluster(bootstrapConfig).Error()
}

func (wc *WormholeConnector) SetupRaft(sigchan chan os.Signal) error {
	rf, err := wc.getNewRaft()
	if err != nil {
		return fmt.Errorf("unable to get new raft: %v", err)
	}
	wc.rf = rf

	if err := wc.bootstrapRaft(wc.rf); err != nil {
		return fmt.Errorf("unable to bootstrap raft cluster: %v", err)
	}

	ticker := time.NewTicker(3 * time.Second)

	for {
		select {
		case <-sigchan:
			// catch SIGTERM to quit immediately
			return nil
		case <-ticker.C:
			if err := lib.ProbeRaft(wc.rf, wc.raftAddr); err != nil {
				return fmt.Errorf("unable to probe raft peers: %v", err)
			}

		case ev := <-wc.serfEvents:
			if err := lib.ProbeSerf(wc.rf, ev); err != nil {
				return fmt.Errorf("unable to probe serf peers: %v", err)
			}
		}
	}
}
