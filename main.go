package main

import (
	"flag"
	"fmt"
	"github.com/ehazlett/docker-hive/server"
	"github.com/goraft/raft"
	"log"
	"math/rand"
	"os"
	"time"
)

const VERSION string = "0.0.1"

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
	flag.StringVar(&host, "h", "", "Node hostname")
	flag.IntVar(&port, "p", 4500, "Port")
	flag.StringVar(&join, "join", "", "host:port of leader to join")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [arguments] <data-path> \n", os.Args[0])
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

	// Set the data directory.
	if flag.NArg() == 0 {
		flag.Usage()
		log.Fatal("Data path argument required")
	}
	path := flag.Arg(0)
	if err := os.MkdirAll(path, 0744); err != nil {
		log.Fatalf("Unable to create path: %v", err)
	}

	log.SetFlags(log.LstdFlags)
	log.Printf("Docker Cluster %s\n", VERSION)
	s := server.New(path, host, port, dockerPath, join)

	log.Fatal(s.ListenAndServe(join))
}
