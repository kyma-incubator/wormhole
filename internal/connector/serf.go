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
	"strings"
	"time"

	bolt "github.com/coreos/bbolt"
	"github.com/hashicorp/serf/serf"
	log "github.com/sirupsen/logrus"

	"github.com/kyma-incubator/wormhole/lib"
)

var (
	defaultSerfDbFile   = "serf.db"
	defaultBucketName   = "SERFDB"
	defaultKeyPeers     = "PEERS"
	defaultSerfChannels = 16
)

// WormholeSerf holds runtime information for Serf, such as database,
// events, peers, and TCP transport information.
type WormholeSerf struct {
	wc *WormholeConnector

	logWriter  *os.File
	logger     *log.Entry
	serfDB     *lib.SerfDB
	serfEvents chan serf.Event
	serfPeers  []lib.SerfPeer
	serfAddr   string
	serfPort   int
	sf         *serf.Serf
}

// NewWormholeSerf returns a new wormhole serf object, which holds e.g.,
// database, events, peers, and TCP transport information.
func NewWormholeSerf(pWc *WormholeConnector, sPeers []lib.SerfPeer, sAddr string, sPort int) (*WormholeSerf, error) {
	ws := &WormholeSerf{
		wc: pWc,

		serfPeers: sPeers,
		serfPort:  sPort,
		serfAddr:  sAddr,
	}

	id := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s:%d", ws.serfAddr, ws.serfPort))))

	serfDataDir := filepath.Join(ws.wc.dataDir, "serf", id)
	if err := os.MkdirAll(serfDataDir, os.FileMode(0755)); err != nil {
		return nil, fmt.Errorf("unable to create directory %s: %v", serfDataDir, err)
	}

	ws.logger = log.WithFields(log.Fields{"component": "serf"})

	var err error
	logFile := filepath.Join(serfDataDir, "serf.log")
	ws.logWriter, err = os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("unable to open file %s: %v", logFile, err)
	}

	if err := ws.InitSerfDB(filepath.Join(serfDataDir, defaultSerfDbFile)); err != nil {
		return nil, fmt.Errorf("unable to initialize serf db: %v", err)
	}

	ws.serfEvents = make(chan serf.Event, defaultSerfChannels)
	ws.sf, err = lib.GetNewSerf(ws.logWriter, serfDataDir, ws.serfAddr, ws.serfPort, ws.serfEvents)
	if err != nil {
		return nil, fmt.Errorf("unable to get new serf: %v", err)
	}
	ws.logger.Infof("starting member %s", id)

	ws.logger.Infof("listening for serf peers on %s:%d", ws.serfAddr, ws.serfPort)

	return ws, nil
}

// InitSerfDB opens the database, initializes the database with the default
// bucket name.
func (ws *WormholeSerf) InitSerfDB(dbPath string) error {
	if _, err := os.Create(dbPath); err != nil {
		return fmt.Errorf("unable to create an empty file %s: %v", dbPath, err)
	}

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

// SetupSerf makes every given Serf peer join the Serf cluster.
func (ws *WormholeSerf) SetupSerf() error {
	if len(ws.serfPeers) == 0 {
		ws.logger.Debug("empty serf peers list, nothing to do.")
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

	ws.logger.Infof("successfully joined %d peers: %s", numJoined, strings.Join(addrs, ","))
	return nil
}

// Shutdown destroys everything for Serf before shutting down the wormhole
// connector.
func (ws *WormholeSerf) Shutdown() {
	if err := ws.serfDB.BoltDB.Close(); err != nil {
		ws.logger.Warnln("cannot close serf DB")
	}
	ws.logWriter.Close()
}

// GetPeerAddrs returns a list of IP addresses of Serf peers.
func (ws *WormholeSerf) GetPeerAddrs() []string {
	peers := []string{}
	for _, p := range ws.serfPeers {
		peers = append(peers, p.Address)
	}
	return peers
}
