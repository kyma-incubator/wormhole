package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
	"github.com/hashicorp/serf/serf"
)

var (
	members  string
	serfPort int
	bindLock sync.Mutex
	bindNum  byte = 10
)

func init() {
	flag.StringVar(&members, "members", "", "127.0.0.1:1111,127.0.0.1:2222")
	flag.IntVar(&serfPort, "serfPort", 0, "1111")
}

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

func getFirstLocalIP() (net.IP, error) {
	bindLock.Lock()
	defer bindLock.Unlock()

	result := net.IPv4(127, 0, 0, bindNum)
	bindNum++
	if bindNum > 255 {
		return net.IP{}, fmt.Errorf("cannot find a free slot for IP address")
	}

	return result, nil
}

func getNewSerf(bindAddr string, serfEvents chan serf.Event) (*serf.Serf, error) {
	memberlistConfig := memberlist.DefaultLANConfig()
	memberlistConfig.BindAddr = bindAddr
	memberlistConfig.BindPort = serfPort
	memberlistConfig.LogOutput = os.Stdout

	serfConfig := serf.DefaultConfig()
	serfConfig.NodeName = fmt.Sprintf("%s:%d", bindAddr, serfPort)
	serfConfig.EventCh = serfEvents
	serfConfig.MemberlistConfig = memberlistConfig
	serfConfig.LogOutput = os.Stdout

	s, err := serf.Create(serfConfig)
	if err != nil {
		//         log.Fatal(err)
		return nil, err
	}

	return s, nil
}

func getNewRaft(dataDir, bindAddr, raftAddr string, raftPort int) (*raft.Raft, error) {
	raftDBPath := filepath.Join(dataDir, "raft.db")
	raftDB, err := raftboltdb.NewBoltStore(raftDBPath)
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

func main() {
	flag.Parse()

	var peers []string

	if members != "" {
		peers = strings.Split(members, ",")
	}

	ip, err := getFirstLocalIP()
	if err != nil {
		log.Fatal(err)
	}

	serfEvents := make(chan serf.Event, 16)
	s, err := getNewSerf(ip.String(), serfEvents)
	if err != nil {
		log.Fatal(err)
	}

	// Join an existing cluster by specifying at least one known member.
	if len(peers) > 0 {
		_, err = s.Join(peers, false)
		if err != nil {
			log.Fatal(err)
		}
	}

	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	raftPort := serfPort + 1
	id := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s:%d", ip, raftPort))))
	dataDir := filepath.Join(workDir, id)
	err = os.RemoveAll(dataDir + "/")
	if err != nil {
		log.Fatal(err)
	}

	err = os.MkdirAll(dataDir, 0777)
	if err != nil {
		log.Fatal(err)
	}

	raftAddr := ip.String() + ":" + strconv.Itoa(raftPort)
	r, err := getNewRaft(dataDir, ip.String(), raftAddr, raftPort)
	if err != nil {
		log.Fatal(err)
	}

	bootstrapConfig := raft.Configuration{
		Servers: []raft.Server{
			{
				Suffrage: raft.Voter,
				ID:       raft.ServerID(raftAddr),
				Address:  raft.ServerAddress(raftAddr),
			},
		},
	}

	// Add known peers to bootstrap
	for _, node := range peers {
		if node == raftAddr {
			continue
		}

		bootstrapConfig.Servers = append(bootstrapConfig.Servers, raft.Server{
			Suffrage: raft.Voter,
			ID:       raft.ServerID(node),
			Address:  raft.ServerAddress(node),
		})
	}

	f := r.BootstrapCluster(bootstrapConfig)
	if err := f.Error(); err != nil {
		log.Fatalf("error bootstrapping: %s", err)
	}

	ticker := time.NewTicker(3 * time.Second)

	for {
		select {
		case <-ticker.C:
			future := r.VerifyLeader()

			fmt.Printf("Showing peers known by %s:\n", raftAddr)
			if err = future.Error(); err != nil {
				fmt.Println("Node is a follower")
			} else {
				fmt.Println("Node is leader")
			}

			cfuture := r.GetConfiguration()
			if err = cfuture.Error(); err != nil {
				log.Fatalf("error getting config: %s", err)
			}

			configuration := cfuture.Configuration()
			for _, server := range configuration.Servers {
				fmt.Println(server.Address)
			}

		case ev := <-serfEvents:
			leader := r.VerifyLeader()
			if memberEvent, ok := ev.(serf.MemberEvent); ok {
				for _, member := range memberEvent.Members {
					changedPeer := member.Addr.String() + ":" + strconv.Itoa(int(member.Port+1))
					if memberEvent.EventType() == serf.EventMemberJoin {
						if leader.Error() == nil {
							f := r.AddVoter(raft.ServerID(changedPeer), raft.ServerAddress(changedPeer), 0, 0)
							if f.Error() != nil {
								log.Fatalf("error adding voter: %s", err)
							}
						}
					} else if memberEvent.EventType() == serf.EventMemberLeave ||
						memberEvent.EventType() == serf.EventMemberFailed ||
						memberEvent.EventType() == serf.EventMemberReap {

						if leader.Error() == nil {
							f := r.RemoveServer(raft.ServerID(changedPeer), 0, 0)

							if f.Error() != nil {
								log.Fatalf("error removing server: %s", err)
							}
						}
					}
				}
			}
		}
	}
}
