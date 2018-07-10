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
	"log"
	"os"
	"path/filepath"
	"time"

	bolt "github.com/coreos/bbolt"

	"github.com/hashicorp/serf/serf"
	"github.com/kinvolk/wormhole-connector/lib"
)

var (
	defaultSerfDbFile   string = "serf.db"
	defaultBucketName   string = "SERFDB"
	defaultKeyPeers     string = "PEERS"
	defaultSerfChannels int    = 16
)

type WormholeSerf struct {
	wc *WormholeConnector

	serfDB     *lib.SerfDB
	serfEvents chan serf.Event
	serfPeers  []lib.SerfPeer
	serfPort   int
	sf         *serf.Serf
}

func NewWormholeSerf(pWc *WormholeConnector, sPeers []lib.SerfPeer, sPort int) *WormholeSerf {
	ws := &WormholeSerf{
		wc: pWc,

		serfPeers: sPeers,
		serfPort:  sPort,
	}

	id := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s:%d", ws.wc.localAddr, ws.serfPort))))

	serfDataDir := filepath.Join(ws.wc.dataDir, "serf", id)
	if err := os.MkdirAll(serfDataDir, os.FileMode(0755)); err != nil {
		log.Printf("unable to create directory %s: %v", serfDataDir, err)
		return nil
	}

	if err := ws.InitSerfDB(filepath.Join(serfDataDir, defaultSerfDbFile)); err != nil {
		log.Printf("unable to initialize serf db: %v", err)
		return nil
	}

	var err error
	ws.serfEvents = make(chan serf.Event, defaultSerfChannels)
	ws.sf, err = lib.GetNewSerf(ws.wc.localAddr, ws.serfPort, ws.serfEvents)
	if err != nil {
		log.Printf("unable to get new serf: %v", err)
		return nil
	}

	return ws
}

func (ws *WormholeSerf) InitSerfDB(dbPath string) error {
	boltdb, err := bolt.Open(dbPath, 0600, &bolt.Options{
		Timeout: 2 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("unable to create serf database: %v", err)
	}

	err = boltdb.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(defaultBucketName)); err != nil {
			return fmt.Errorf("unable to create root bucket: %v", err)
		}
		return nil
	})

	ws.serfDB = &lib.SerfDB{
		BoltDB:         boltdb,
		SerfBucketName: defaultBucketName,
		SerfKeyPeers:   defaultKeyPeers,
	}

	return nil
}

func (ws *WormholeSerf) SetupSerf() error {
	if len(ws.serfPeers) == 0 {
		log.Println("empty serf peers list, nothing to do.")
		return nil
	}

	// Join an existing cluster by specifying at least one known member.
	addrs := []string{}
	for _, p := range ws.serfPeers {
		addrs = append(addrs, p.Address)
	}
	numJoined, err := ws.sf.Join(addrs, false)
	if err != nil {
		return fmt.Errorf("unable to join an existing serf cluster: %v", err)
	}

	log.Printf("successfully joined %d peers\n", numJoined)
	return nil
}

func (ws *WormholeSerf) Shutdown() {
	if err := ws.serfDB.BoltDB.Close(); err != nil {
		fmt.Printf("cannot close DB\n")
	}
}

func (ws *WormholeSerf) GetPeerAddrs() []string {
	peers := []string{}
	for _, p := range ws.serfPeers {
		peers = append(peers, p.Address)
	}
	return peers
}
