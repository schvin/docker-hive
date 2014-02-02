package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/dotcloud/docker"
	"github.com/goraft/raft"
	"github.com/ehazlett/docker-cluster/server"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"sync"
	"time"
)

const VERSION string = "0.1.0"

var (
	dockerPath    string
	runInterval   int
	registerAgent bool
	version       bool
	port          int
	verbose       bool
	trace         bool
	debug         bool
	host          string
	join          string
)

type (
	AgentData struct {
		Key string `json:"key"`
	}

	ContainerData struct {
		Container docker.APIContainers
		Meta      *docker.Container
	}

	Job struct {
		Name string
		Path string
		Data interface{}
	}

	Image struct {
		Id          string
		Created     int
		RepoTags    []string
		Size        int
		VirtualSize int
	}
)

func newClient(path string) (*httputil.ClientConn, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	return httputil.NewClientConn(conn, nil), nil
}

func updater(jobs <-chan *Job, group *sync.WaitGroup, s *server.Server) {
	group.Add(1)
	defer group.Done()
	//client := &http.Client{}
	for obj := range jobs {
		buf := bytes.NewBuffer(nil)
		if err := json.NewEncoder(buf).Encode(obj.Data); err != nil {
			log.Println(err)
			continue
		}
		key := fmt.Sprintf("%s:%s", obj.Name, s.RaftServer.Name())
		path, err := s.Router.Get("db").URL("key", key)
		if err != nil {
			log.Printf("Error reversing the URL for %s: %s\n", obj.Name, err)
		}
		if err != nil {
			log.Printf("Error parsing data: %s\n", err)
		}
		url := fmt.Sprintf("%s%s", s.ConnectionString(), path.String())
		resp, err := http.Post(url, "text/plain", buf)
		if err != nil {
			log.Printf("Error posting to the API: %s\n", err)
		}
		// check for non-master redirect
		switch resp.StatusCode {
		case http.StatusCreated:
			// ignore success
		case http.StatusMovedPermanently:
			// re-post to leader
			url = resp.Header.Get("Location")
			if err := json.NewEncoder(buf).Encode(obj.Data); err != nil {
				log.Printf("Error encoding container JSON: %s", err)
				continue
			}
			resp, err := http.Post(url, "text/plain", buf)
			if err != nil {
				log.Printf("Error posting to the API: %s\n", err)
			}
			defer resp.Body.Close()
		default:
			contents, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Unable to parse API response: %s\n", err)
			}
			defer resp.Body.Close()
			log.Printf("Error creating via API: HTTP %s: %s\n", strconv.Itoa(resp.StatusCode), string(contents))
		}
		defer resp.Body.Close()
	}
}

func getContainers() []*docker.APIContainers {
	path := fmt.Sprintf("/containers/json?all=1")
	c, err := newClient(dockerPath)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := c.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var containers []*docker.APIContainers
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&containers); err != nil {
			log.Fatal(err)
		}
	}
	return containers
}

func inspectContainer(id string) *docker.Container {
	path := fmt.Sprintf("/containers/%s/json?all=1", id)
	c, err := newClient(dockerPath)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := c.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var container *docker.Container
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&container); err != nil {
			log.Fatal(err)
		}
	}
	return container
}

func getImages() []*Image {
	path := "/images/json?all=0"
	c, err := newClient(dockerPath)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := c.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var images []*Image
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&images); err != nil {
			log.Fatal(err)
		}
	}
	return images
}

func pushContainers(jobs chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	containers := getContainers()

	jobs <- &Job{
		Path: "/agent/containers/",
		Data: containers,
		Name: "containers",
	}
}

func pushImages(jobs chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	images := getImages()
	jobs <- &Job{
		Path: "/agent/images/",
		Data: images,
		Name: "images",
	}
}

func update(d time.Duration, s *server.Server) {
	var (
		updaterGroup = &sync.WaitGroup{}
		pushGroup    = &sync.WaitGroup{}
		// create chan with a 2 buffer, we use a 2 buffer to sync the go routines so that
		// no more than two messages are being send to the server at one time
		jobs = make(chan *Job, 2)
	)

	go updater(jobs, updaterGroup, s)

	for _ = range time.Tick(d) {
		go pushContainers(jobs, pushGroup)
		go pushImages(jobs, pushGroup)
		pushGroup.Wait()
	}

	// wait for all request to finish processing before returning
	updaterGroup.Wait()
}

func init() {
	flag.StringVar(&dockerPath, "docker", "/var/run/docker.sock", "Path to Docker socket")
	flag.IntVar(&runInterval, "interval", 5, "Run interval")
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

	duration, err := time.ParseDuration(fmt.Sprintf("%ds", runInterval))
	if err != nil {
		log.Fatal(err)
	}

	rand.Seed(time.Now().UnixNano())

	// Setup commands.
	raft.RegisterCommand(&server.WriteCommand{})
	raft.RegisterCommand(&server.ActionCommand{})

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
	log.Printf("Docker Cluster: %s)\n", VERSION)
	s := server.New(path, host, port)
	go update(duration, s)

	log.Fatal(s.ListenAndServe(join))
}
