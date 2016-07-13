// Copyright 2015 Sorint.lab
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/alecthomas/kingpin"
	"github.com/gravitational/stolon/pkg/util"
	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
)

const (
	EnvStoreEndpoints = "STOLONCTL_STORE_ENDPOINTS"
	EnvStoreBackend   = "STOLONCTL_STORE_BACKEND"
	EnvStoreKey       = "STOLONCTL_STORE_KEY"
	EnvStoreCACert    = "STOLONCTL_STORE_CA_CERT"
	EnvStoreCert      = "STOLONCTL_STORE_CERT"
	OutputJSON        = "json"
)

func run() error {
	app := kingpin.New("stolonctl", "Cluster-Native K8s deployment manager")

	var debug bool
	app.Flag("debug", "Enable verbose logging to stderr").
		Short('d').
		BoolVar(&debug)

	if debug {
		util.InitLoggerDebug()

	} else {
		util.InitLoggerCLI()
	}

	var cfg config
	app.Flag("store-endpoints",
		"a comma-delimited list of store endpoints (defaults: 127.0.0.1:2379 for etcd, 127.0.0.1:8500 for consul)").
		Envar(EnvStoreEndpoints).StringVar(&cfg.storeEndpoints)

	app.Flag("store-backend", "store backend type (etcd or consul)").
		Envar(EnvStoreBackend).StringVar(&cfg.storeBackend)

	app.Flag(
		"store-cert",
		"path to the client server TLS cert file").
		Envar(EnvStoreCert).StringVar(&cfg.storeCertFile)

	app.Flag("store-key", "path to the client server TLS key file").
		Envar(EnvStoreKey).StringVar(&cfg.storeKeyFile)

	app.Flag("store-cacert", "path to the client server TLS trusted CA key file").
		Envar(EnvStoreCACert).StringVar(&cfg.storeCACertFile)

	cmdCluster := app.Command("cluster", "operations on existing cluster")

	// print config
	cmdClusterConfig := cmdCluster.Command("config", "print configuration for cluster")
	cmdClusterConfigName := cmdClusterConfig.Arg("cluster-name", "cluster name").Required().String()

	// patch config
	cmdClusterPatch := cmdCluster.Command("patch", "patch configuration for cluster")
	cmdClusterPatchName := cmdClusterPatch.Arg("cluster-name", "cluster name").Required().String()
	cmdClusterPatchFile := cmdClusterPatch.Flag("file", "patch configuration for cluster").Short('f').String()

	// replace config
	cmdClusterReplace := cmdCluster.Command("replace", "replace configuration for cluster")
	cmdClusterReplaceName := cmdClusterReplace.Arg("cluster-name", "cluster name").Required().String()
	cmdClusterReplaceFile := cmdClusterReplace.Flag("file", "replace configuration for cluster").Short('f').String()

	// print status
	cmdClusterStatus := cmdCluster.Command("status", "print cluster status")
	cmdClusterStatusName := cmdClusterStatus.Arg("cluster-name", "cluster name").Required().String()
	cmdClusterStatusMasterOnly := cmdClusterStatus.Flag("master", "limit output to master only").Default("false").Bool()
	cmdClusterStatusOutputJson := cmdClusterStatus.Flag("json", "format output to json").Default("false").Bool()

	// list clusters
	cmdClusterList := cmdCluster.Command("list", "list clusters")

	cmd, err := app.Parse(os.Args[1:])
	if err != nil {
		return trace.Wrap(err)
	}

	ctl, err := newClient(cfg)
	if err != nil {
		return trace.Wrap(err)
	}

	switch cmd {
	case cmdClusterConfig.FullCommand():
		return printConfig(ctl, *cmdClusterConfigName)
	case cmdClusterPatch.FullCommand():
		return patchConfig(ctl, *cmdClusterPatchName, *cmdClusterPatchFile, os.Args[len(os.Args)-1] == "-")
	case cmdClusterReplace.FullCommand():
		return replaceConfig(ctl, *cmdClusterReplaceName, *cmdClusterReplaceFile, os.Args[len(os.Args)-1] == "-")
	case cmdClusterStatus.FullCommand():
		return status(ctl, *cmdClusterStatusName, *cmdClusterStatusMasterOnly, *cmdClusterStatusOutputJson)
	case cmdClusterList.FullCommand():
		return list(ctl)
	}

	return nil
}

func printConfig(clt *client, clusterName string) error {
	cluster, err := clt.getCluster(clusterName)
	if err != nil {
		return trace.Wrap(err)
	}
	cfg, err := cluster.Config()
	if err != nil {
		return trace.Wrap(err)
	}
	data, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return trace.Wrap(err, "failed to marshal configuration")
	}
	fmt.Fprintln(os.Stdout, data)

	return nil
}

func patchConfig(clt *client, clusterName string, patchFile string, readStdin bool) error {
	data, err := readFile(patchFile, readStdin)
	if err != nil {
		return trace.Wrap(err)
	}
	cluster, err := clt.getCluster(clusterName)
	if err != nil {
		return trace.Wrap(err)
	}
	err = cluster.PatchConfig(data)

	return trace.Wrap(err)
}

func replaceConfig(clt *client, clusterName string, replaceFile string, readStdin bool) error {
	data, err := readFile(replaceFile, readStdin)
	if err != nil {
		return trace.Wrap(err)
	}
	cluster, err := clt.getCluster(clusterName)
	if err != nil {
		return trace.Wrap(err)
	}
	err = cluster.ReplaceConfig(data)

	return trace.Wrap(err)
}

func readFile(fileName string, readStdin bool) ([]byte, error) {
	if (readStdin && fileName != "") || (!readStdin && fileName == "") {
		return nil, trace.BadParameter("need either file to read from or readStdin option")
	}
	var config []byte
	var err error
	if readStdin {
		config, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			return nil, trace.Wrap(err, "cannot read config file from stdin")
		}
	} else {
		config, err = ioutil.ReadFile(fileName)
		if err != nil {
			return nil, trace.Wrap(err, "can not read file")
		}
	}

	return config, trace.Wrap(err)
}

func list(clt *client) error {
	clusters, err := clt.Clusters()
	if err != nil {
		return trace.Wrap(err)
	}
	for _, cluster := range clusters {
		fmt.Fprintln(os.Stdout, cluster)
	}

	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Error(trace.DebugReport(err))
		os.Exit(1)
	}
}
