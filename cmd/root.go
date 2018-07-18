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

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/kinvolk/wormhole-connector/internal/connector"
)

var cfgFile string

// RootCmd represents the base command when called without any subcommands
var (
	RootCmd = &cobra.Command{
		Use:   "wormhole-connector",
		Short: "Connect Kyma to the outside",
		Long:  `wormhole-connector is a distributed connectivity helper for Kyma clusters.`,
		Run:   runWormholeConnector,
	}

	defaultDataDir = fmt.Sprintf("%s/.config/wormhole-connector", os.Getenv("HOME"))

	flagDataDir         string
	flagKymaServer      string
	flagTimeout         time.Duration
	flagSerfMemberAddrs string
	flagSerfPort        int
	flagRaftPort        int
	flagLocalAddr       string
	flagStandalone      bool
)

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	if err := RootCmd.Execute(); err != nil {
		return err
	}

	return nil
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/wormhole-connector/connector.yaml)")
	RootCmd.PersistentFlags().StringVar(&flagKymaServer, "kyma-server", "localhost:9090", "Kyma server address")
	RootCmd.PersistentFlags().DurationVar(&flagTimeout, "timeout", 5*time.Minute, "Timeout for the HTTP/2 connection")
	RootCmd.PersistentFlags().StringVar(&flagSerfMemberAddrs, "serf-member-addrs", "", "a set of IP:Port pairs of each Serf member")
	RootCmd.PersistentFlags().IntVar(&flagSerfPort, "serf-port", 1111, "port number on which Serf listens (default is 1111)")
	RootCmd.PersistentFlags().IntVar(&flagRaftPort, "raft-port", 1112, "port number on which Raft listens (default is 1112)")
	RootCmd.PersistentFlags().StringVar(&flagLocalAddr, "local-addr", "127.0.0.1:8080", "address to bind")
	RootCmd.PersistentFlags().StringVar(&flagDataDir, "data-dir", defaultDataDir, "data directory to store state")
	RootCmd.PersistentFlags().BoolVar(&flagStandalone, "standalone", false, "whether to run alone without getting involved in serf/raft (default false)")

	viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("kymaServer", RootCmd.PersistentFlags().Lookup("kyma-server"))
	viper.BindPFlag("timeout", RootCmd.PersistentFlags().Lookup("timeout"))
	viper.BindPFlag("serf.memberAddrs", RootCmd.PersistentFlags().Lookup("serf-member-addrs"))
	viper.BindPFlag("serf.port", RootCmd.PersistentFlags().Lookup("serf-port"))
	viper.BindPFlag("raft.port", RootCmd.PersistentFlags().Lookup("raft-port"))
	viper.BindPFlag("localAddr", RootCmd.PersistentFlags().Lookup("local-addr"))
	viper.BindPFlag("dataDir", RootCmd.PersistentFlags().Lookup("data-dir"))
	viper.BindPFlag("standalone", RootCmd.PersistentFlags().Lookup("standalone"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" { // enable ability to specify config file via flag
		viper.SetConfigFile(cfgFile)
	}

	viper.SetConfigName("connector")                        // name of config file (without extension)
	viper.AddConfigPath("/etc/wormhole-connector")          // adding home directory as first search path
	viper.AddConfigPath("$HOME/.config/wormhole-connector") // adding home directory as first search path
	viper.AutomaticEnv()                                    // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

func runWormholeConnector(cmd *cobra.Command, args []string) {
	config := connector.WormholeConnectorConfig{
		KymaServer:      viper.GetString("kymaServer"),
		RaftPort:        viper.GetInt("raft.port"),
		LocalAddr:       viper.GetString("localAddr"),
		SerfMemberAddrs: viper.GetString("serf.memberAddrs"),
		SerfPort:        viper.GetInt("serf.port"),
		Timeout:         viper.GetDuration("timeout"),
		DataDir:         viper.GetString("dataDir"),
		Standalone:      viper.GetBool("standalone"),
	}

	term := make(chan os.Signal, 2)
	signal.Notify(term, os.Interrupt)

	w, err := connector.NewWormholeConnector(config)
	if err != nil {
		log.Fatal(err)
	}

	w.ListenAndServeTLS("server.crt", "server.key")

	if config.Standalone {
		log.Debug("Getting into a standalone mode.")
		<-term
	} else {
		if err := w.SetupSerfRaft(); err != nil {
			log.Fatal(err)
		}

		if err := w.ProbeSerfRaft(term); err != nil {
			log.Fatal(err)
		}
	}

	log.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Shutdown(ctx)

	os.Exit(0)
}
