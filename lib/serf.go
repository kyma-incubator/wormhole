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
	"fmt"
	"os"

	bolt "github.com/coreos/bbolt"
	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/serf/serf"
)

type SerfDB struct {
	BoltDB         *bolt.DB
	SerfKeyPeers   string
	SerfBucketName string
}

type SerfPeer struct {
	Address  string `json:"address,omitempty"`
	PeerName string `json:"nodename,omitempty"`
}

func (db *SerfDB) GetPeer(key string) SerfPeer {
	bdb := db.BoltDB
	var resPeer SerfPeer

	err := bdb.View(func(tx *bolt.Tx) error {
		if val := tx.Bucket([]byte(db.SerfBucketName)).Get([]byte(key)); val != nil {
			json.Unmarshal([]byte(val), &resPeer)
		}
		return nil
	})
	if err != nil {
		return SerfPeer{}
	}

	return resPeer
}

func (db *SerfDB) SetPeer(key string, newPeer SerfPeer) {
	bdb := db.BoltDB

	if err := bdb.Update(func(tx *bolt.Tx) error {
		peerBytes, err := json.Marshal(newPeer)
		if err != nil {
			return err
		}

		tx.Bucket([]byte(db.SerfBucketName)).Put([]byte(key), peerBytes)
		return nil
	}); err != nil {
		return
	}
}

func (db *SerfDB) DeletePeer(key string, newPeer SerfPeer) {
	bdb := db.BoltDB

	if err := bdb.Update(func(tx *bolt.Tx) error {
		tx.Bucket([]byte(db.SerfBucketName)).Delete([]byte(key))
		return nil
	}); err != nil {
		return
	}
}

func GetNewSerf(serfAddr string, serfPort int, serfEvents chan serf.Event) (*serf.Serf, error) {
	memberlistConfig := memberlist.DefaultLANConfig()
	memberlistConfig.BindAddr = serfAddr
	memberlistConfig.BindPort = serfPort
	memberlistConfig.LogOutput = os.Stdout

	serfConfig := serf.DefaultConfig()
	serfConfig.NodeName = fmt.Sprintf("%s:%d", serfAddr, serfPort)
	serfConfig.EventCh = serfEvents
	serfConfig.MemberlistConfig = memberlistConfig
	serfConfig.LogOutput = os.Stdout

	s, err := serf.Create(serfConfig)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func IsMemberEventFailed(event serf.MemberEvent) bool {
	switch event.EventType() {
	case serf.EventMemberLeave:
		fallthrough
	case serf.EventMemberFailed:
		fallthrough
	case serf.EventMemberReap:
		return true
	}
	return false
}
