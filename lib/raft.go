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

// GetNewRaft returns the default Raft object, which includes for example,
// a local ID, FSM(Finite State Machine), boltdb stores for logstore and
// snapshotstore, and a TCP transport.
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
