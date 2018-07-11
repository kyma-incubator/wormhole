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

// SerfDB holds necessary infos for accessing to Serf's BoltDB.
// BoltDB is a pointer for accessing directly to the underlying database.
// SerfKeyPeers holds a key string to retrieve each entry for serf peers.
// SerfBucketName holds a bucket name used at the very first step of DB access.
type SerfDB struct {
	BoltDB         *bolt.DB
	SerfKeyPeers   string
	SerfBucketName string
}

// SerfPeer holds a name and an IP address of each Serf peer.
type SerfPeer struct {
	Address  string `json:"address,omitempty"`
	PeerName string `json:"nodename,omitempty"`
}

func (db *SerfDB) getPeer(key string) SerfPeer {
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

func (db *SerfDB) setPeer(key string, newPeer SerfPeer) {
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

func (db *SerfDB) deletePeer(key string, newPeer SerfPeer) {
	bdb := db.BoltDB

	if err := bdb.Update(func(tx *bolt.Tx) error {
		tx.Bucket([]byte(db.SerfBucketName)).Delete([]byte(key))
		return nil
	}); err != nil {
		return
	}
}

// GetNewSerf returns the default Serf object, which includes for example,
// memberlist configs (like bind address, bind port), node name, event channel.
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

// IsMemberEventFailed returns true if the given Serf member event is
// one of the following event types: leave, failed, or reap. It will be
// typically used for synchronizing status of a remote peer that has gone
// out of the Serf peers.
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
