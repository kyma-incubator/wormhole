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
	defaultSerfDbFile string = "serf.db"
	defaultBucketName string = "SERFDB"
	defaultKeyPeers   string = "PEERS"
)

func (w *WormholeConnector) InitSerfDB(dbPath string) error {
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

	w.serfDB = &lib.SerfDB{
		BoltDB:         boltdb,
		SerfBucketName: defaultBucketName,
		SerfKeyPeers:   defaultKeyPeers,
	}

	return nil
}

func (w *WormholeConnector) SetupSerf() error {
	id := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s:%d", w.localAddr, w.serfPort))))

	serfDataDir := filepath.Join(w.workDir, "tmp/serf", id)
	if err := os.MkdirAll(serfDataDir, os.FileMode(0755)); err != nil {
		return fmt.Errorf("unable to create directory %s: %v", serfDataDir, err)
	}

	if err := w.InitSerfDB(filepath.Join(serfDataDir, defaultSerfDbFile)); err != nil {
		return fmt.Errorf("unable to initialize serf db: %v", err)
	}

	w.serfEvents = make(chan serf.Event, 16)
	sf, err := lib.GetNewSerf(w.localAddr, w.serfPort, w.serfEvents)
	if err != nil {
		return fmt.Errorf("unable to get new serf: %v", err)
	}

	if len(w.serfPeers) == 0 {
		log.Println("empty serf peers list, nothing to do.")
		return nil
	}

	// Join an existing cluster by specifying at least one known member.
	addrs := []string{}
	for _, p := range w.serfPeers {
		addrs = append(addrs, p.Address)
	}
	numJoined, err := sf.Join(addrs, false)
	if err != nil {
		return fmt.Errorf("unable to join an existing serf cluster: %v", err)
	}

	log.Printf("successfully joined %d peers\n", numJoined)
	return nil
}
