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

package lib

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	boltdb "github.com/hashicorp/raft-boltdb"
)

type fsm struct {
}

func (f *fsm) Apply(*raft.Log) interface{} {
	return nil
}

func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	return nil, nil
}

func (f *fsm) Restore(io.ReadCloser) error {
	return nil
}

func GetNewRaft(dataDir, raftAddr string, raftPort int) (*raft.Raft, error) {
	raftDBPath := filepath.Join(dataDir, "raft.db")
	raftDB, err := boltdb.NewBoltStore(raftDBPath)
	if err != nil {
		return nil, err
	}

	snapshotStore, err := raft.NewFileSnapshotStore(dataDir, 1, os.Stdout)
	if err != nil {
		return nil, err
	}

	trans, err := raft.NewTCPTransport(raftAddr, nil, 3, 10*time.Second, os.Stdout)
	if err != nil {
		return nil, err
	}

	c := raft.DefaultConfig()
	c.LogOutput = os.Stdout
	c.LocalID = raft.ServerID(raftAddr)

	r, err := raft.NewRaft(c, &fsm{}, raftDB, raftDB, snapshotStore, trans)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func ProbeRaft(rf *raft.Raft, raftAddr string) error {
	future := rf.VerifyLeader()

	fmt.Printf("Showing peers known by %s:\n", raftAddr)
	if err := future.Error(); err != nil {
		fmt.Println("Node is a follower")
	} else {
		fmt.Println("Node is leader")
	}

	cfuture := rf.GetConfiguration()
	if err := cfuture.Error(); err != nil {
		return fmt.Errorf("error getting config: %s", err)
	}

	configuration := cfuture.Configuration()
	for _, server := range configuration.Servers {
		fmt.Println(server.Address)
	}

	return nil
}
