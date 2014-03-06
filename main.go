/*
   Copyright Evan Hazlett

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/ehazlett/docker-hive/server"
	"github.com/ehazlett/docker-hive/third_party/github.com/goraft/raft"
)

const VERSION string = "0.0.2"

var (
	dockerPath string
	version    bool
	port       int
	verbose    bool
	trace      bool
	debug      bool
	host       string
	join       string
)

func init() {
	flag.StringVar(&dockerPath, "docker", "/var/run/docker.sock", "Path to Docker socket")
	flag.BoolVar(&version, "version", false, "Shows version")
	flag.BoolVar(&verbose, "v", false, "Enable verbose logging")
	flag.BoolVar(&trace, "trace", false, "Raft trace debugging")
	flag.BoolVar(&debug, "debug", false, "Raft debugging")
	flag.StringVar(&host, "n", "", "Node hostname")
	flag.IntVar(&port, "p", 4500, "Port")
	flag.StringVar(&join, "join", "", "host:port of leader to join")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [arguments] [data-path] \n", os.Args[0])
		flag.PrintDefaults()
	}
}

func main() {
	log.SetFlags(0)
	flag.Parse()
	if version {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	if verbose {
		log.Print("Verbose logging enabled.")
	}
	if trace {
		raft.SetLogLevel(raft.Trace)
		log.Print("Raft trace debugging enabled.")
	} else if debug {
		raft.SetLogLevel(raft.Debug)
		log.Print("Raft debugging enabled.")
	}

	rand.Seed(time.Now().UnixNano())

	// Setup commands.
	raft.RegisterCommand(&server.WriteCommand{})
	raft.RegisterCommand(&server.ActionCommand{})
	raft.RegisterCommand(&server.SyncCommand{})

	var path string
	// Set the data directory.
	if flag.NArg() == 0 {
		// create temp dir
		p, err := ioutil.TempDir(os.TempDir(), "hive_")
		if err != nil {
			log.Fatalf("Unable to create data path: %v", err)
		}
		path = p
	} else {
		path = flag.Arg(0)
		if err := os.MkdirAll(path, 0744); err != nil {
			log.Fatalf("Unable to create data path: %v", err)
		}
	}
	log.Println(path)
	log.SetFlags(log.LstdFlags)
	log.Printf("Docker Hive %s\n", VERSION)
	s := server.New(path, host, port, dockerPath, join)

	waiter, err := s.Start()
	if err != nil {
		log.Fatal(err)
		return
	}
	waiter.Wait()
}
