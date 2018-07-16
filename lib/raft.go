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
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	boltdb "github.com/hashicorp/raft-boltdb"
)

var (
	defaultTransportTimeout = 10 * time.Second
)

// EventsFSM is a Finite State Machine representing events (for now they're
// stored as simple strings).
//
// The raft library uses an FSM as an abstraction to allow the current state of
// the cluster to be replicated to other nodes. We implement the Apply()
// operation defined in the FSM interface to take advantage of this.
//
// Since restoring the state of the FSM must result in the same state as a full
// replay of the raft logs, the raft library can capture the FSM state at a
// point in time and then remove all the logs used to reach that state, this is
// performed automatically to avoid unbound log growth. We implement the
// Snapshot() and Restore() operations defined in the FSM interface and the
// Persist() operation defined in the FSMSnapshot interface to take advantage
// of this.
type EventsFSM struct {
	sync.Mutex

	Events []string
}

func NewEventsFSM() *EventsFSM {
	fsm := new(EventsFSM)

	return fsm
}

const (
	EnqueueCmd = "enqueue"
	DiscardCmd = "discard"
)

type Action struct {
	Cmd   string `json:"cmd"`
	Event string `json:"event"`
}

func (fsm *EventsFSM) HandleAction(a *Action) {
	switch a.Cmd {
	case EnqueueCmd:
		fsm.EnqueueEvent(a.Event)
	case DiscardCmd:
		fsm.DiscardTopEvent()
	}
}

func (f *EventsFSM) Apply(l *raft.Log) interface{} {
	var a Action
	if err := json.Unmarshal(l.Data, &a); err != nil {
		log.Printf("error decoding raft log: %v", err)
		return err
	}

	f.HandleAction(&a)

	return nil
}

func (f *EventsFSM) Snapshot() (raft.FSMSnapshot, error) {
	snap := new(eventsSnapshot)
	snap.events = make([]string, 0, len(f.Events))

	f.Lock()
	for _, e := range f.Events {
		snap.events = append(snap.events, e)
	}
	f.Unlock()

	return snap, nil
}

func (f *EventsFSM) Restore(snap io.ReadCloser) error {
	defer snap.Close()

	d := json.NewDecoder(snap)
	var events []string

	if err := d.Decode(&events); err != nil {
		return err
	}

	f.Lock()
	for _, e := range events {
		f.Events = append(f.Events, e)
	}
	f.Unlock()

	return nil
}

type eventsSnapshot struct {
	events []string
}

func (snap *eventsSnapshot) Persist(sink raft.SnapshotSink) error {
	data, _ := json.Marshal(snap.events)
	_, err := sink.Write(data)
	if err != nil {
		sink.Cancel()
	}
	return err
}

func (snap *eventsSnapshot) Release() {
}

// GetNewRaft returns the default Raft object, which includes for example,
// a local ID, FSM(Finite State Machine), logstore and snapshotstore, and
// a TCP transport.
func GetNewRaft(logWriter *os.File, dataDir, raftAddr string, raftPort int, fsm raft.FSM) (*raft.Raft, error) {
	raftDBPath := filepath.Join(dataDir, "raft.db")
	raftDB, err := boltdb.NewBoltStore(raftDBPath)
	if err != nil {
		return nil, err
	}

	snapshotStore, err := raft.NewFileSnapshotStore(dataDir, 1, logWriter)
	if err != nil {
		return nil, err
	}

	trans, err := raft.NewTCPTransport(raftAddr, nil, 3, defaultTransportTimeout, logWriter)
	if err != nil {
		return nil, err
	}

	c := raft.DefaultConfig()
	c.LogOutput = logWriter
	c.LocalID = raft.ServerID(raftAddr)

	r, err := raft.NewRaft(c, fsm, raftDB, raftDB, snapshotStore, trans)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (fsm *EventsFSM) EnqueueEvent(ev string) {
	fsm.Lock()
	defer fsm.Unlock()

	fsm.Events = append(fsm.Events, ev)
}

func (fsm *EventsFSM) DiscardTopEvent() {
	fsm.Lock()
	defer fsm.Unlock()

	if len(fsm.Events) == 0 {
		return
	}

	fsm.Events = fsm.Events[1:]
}

func (fsm *EventsFSM) TopEvent() string {
	fsm.Lock()
	defer fsm.Unlock()

	if len(fsm.Events) == 0 {
		return ""
	}

	return fsm.Events[0]
}
