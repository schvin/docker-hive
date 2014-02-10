package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dotcloud/docker"
	"github.com/ehazlett/docker-cluster/db"
	"github.com/goraft/raft"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type (
	Job struct {
		Name   string
		Data   interface{}
		Encode bool
	}

	Image struct {
		Id          string
		Created     int
		RepoTags    []string
		Size        int
		VirtualSize int
	}

	Port struct {
		IP          string
		PrivatePort int
		PublicPort  int
		Type        string
	}

	APIContainer struct {
		Id      string
		Created int
		Image   string
		Status  string
		Command string
		Ports   []Port
		Names   []string
	}

	Server struct {
		name       string
		Host       string
		Port       int
		path       string
		httpServer *http.Server
		db         *db.DB
		mutex      sync.RWMutex
		Router     *mux.Router
		RaftServer raft.Server
		DockerPath string
		LeaderURL  string
	}
)

// Creates a new server.
func New(path string, host string, port int, dockerPath string, leader string) *Server {
	s := &Server{
		Host:       host,
		Port:       port,
		path:       path,
		db:         db.New(),
		Router:     mux.NewRouter(),
		DockerPath: dockerPath,
		LeaderURL:  leader,
	}

	// Read existing name or generate a new one.
	if b, err := ioutil.ReadFile(filepath.Join(path, "name")); err == nil {
		s.name = string(b)
	} else {
		s.name = fmt.Sprintf("%07x", rand.Int())[0:7]
		if err = ioutil.WriteFile(filepath.Join(path, "name"), []byte(s.name), 0644); err != nil {
			panic(err)
		}
	}
	return s
}

func (s *Server) newDockerClient() (*httputil.ClientConn, error) {
	conn, err := net.Dial("unix", s.DockerPath)
	if err != nil {
		return nil, err
	}
	return httputil.NewClientConn(conn, nil), nil
}

func (s *Server) syncDocker(interval int) {
	var (
		updaterGroup = &sync.WaitGroup{}
		pushGroup    = &sync.WaitGroup{}
		// create chan with a 2 buffer, we use a 2 buffer to sync the go routines so that
		// no more than two messages are being send to the server at one time
		jobs = make(chan *Job, 2)
	)
	duration, err := time.ParseDuration(fmt.Sprintf("%ds", interval))
	if err != nil {
		log.Fatalf("Unable to parse sync interval: %s", err)
	}

	go s.updater(jobs, updaterGroup)

	for _ = range time.Tick(duration) {
		go s.pushContainers(jobs, pushGroup)
		go s.pushImages(jobs, pushGroup)
		pushGroup.Wait()
	}
	// wait for all request to finish processing before returning
	updaterGroup.Wait()
}

func (s *Server) updater(jobs <-chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	for obj := range jobs {
		buf := bytes.NewBuffer(nil)
		if obj.Encode {
			if err := json.NewEncoder(buf).Encode(obj.Data); err != nil {
				log.Printf("Error encoding to JSON: %s", err)
				continue
			}
		} else {
			buf = bytes.NewBufferString(obj.Data.(string))
		}
		key := obj.Name
		path, err := s.Router.Get("db").URL("key", key)
		if err != nil {
			log.Printf("Error reversing the URL for %s: %s\n", obj.Name, err)
			return
		}
		if err != nil {
			log.Printf("Error parsing data: %s\n", err)
			return
		}
		host := s.GetConnectionString(s.Leader())
		url := fmt.Sprintf("%s%s", host, path.String())
		resp, err := http.Post(url, "text/plain", buf)
		if err != nil {
			log.Printf("Error posting to the API: %s\n", err)
			return
		}
		// check for non-master redirect
		switch resp.StatusCode {
		case http.StatusOK, http.StatusCreated:
			// ignore success
		case http.StatusFound:
			log.Printf("Redirect StatusFound")
			// re-post to leader
			url = resp.Header.Get("Location")
			if err := json.NewEncoder(buf).Encode(obj.Data); err != nil {
				log.Printf("Error encoding container JSON: %s", err)
				return
			}
			res, err := http.Post(url, "text/plain", buf)
			if err != nil {
				log.Printf("Error posting to the API: %s\n", err)
				return
			}
			defer res.Body.Close()
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

func (s *Server) getContainers() []APIContainer {
	path := fmt.Sprintf("/containers/json?all=1")
	c, err := s.newDockerClient()
	defer c.Close()
	if err != nil {
		log.Fatalf("Error connecting to Docker: %s", err)
	}
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Fatalf("Error requesting containers from Docker: %s", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		log.Fatalf("Error requesting containers from Docker: %s", err)
	}
	defer resp.Body.Close()

	var containers []APIContainer
	if resp.StatusCode == http.StatusOK {
		contents, _ := ioutil.ReadAll(resp.Body)
		r := bytes.NewReader(contents)
		d := json.NewDecoder(r)
		if err = d.Decode(&containers); err != nil {
			log.Fatalf("Erroring decoding container JSON: %s", err)
		}
		resp.Body.Close()
	}
	return containers
}

func (s *Server) inspectContainer(id string) *docker.Container {
	path := fmt.Sprintf("/containers/%s/json?all=1", id)
	c, err := s.newDockerClient()
	defer c.Close()
	if err != nil {
		log.Fatalf("Error connecting to Docker: %s", err)
	}
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Fatalf("Error inspecting container from Docker: %s", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		log.Fatalf("Error inspecting container from Docker: %s", err)
	}

	var container *docker.Container
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&container); err != nil {
			log.Fatalf("Erroring decoding container JSON: %s", err)
		}
	}
	resp.Body.Close()
	return container
}

func (s *Server) getImages() []*Image {
	path := "/images/json?all=0"
	c, err := s.newDockerClient()
	if err != nil {
		log.Fatalf("Error connecting to Docker: %s", err)
	}
	defer c.Close()
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		log.Fatalf("Error requesting images from Docker: %s", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		log.Fatalf("Error requesting images from Docker: %s", err)
	}

	var images []*Image
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&images); err != nil {
			log.Fatalf("Erroring decoding image JSON: %s", err)
		}
	}
	resp.Body.Close()
	return images
}

func (s *Server) pushContainers(jobs chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	containers := s.getContainers()
	host := s.RaftServer.Name()
	jobs <- &Job{
		Data:   containers,
		Name:   fmt.Sprintf("containers:%s", host),
		Encode: true,
	}
	for _, container := range containers {
		data := fmt.Sprintf("%s::%s", host, s.GetConnectionString(host))
		jobs <- &Job{
			Data:   data,
			Name:   fmt.Sprintf("container:host:%s", container.Id),
			Encode: false,
		}
		c := s.inspectContainer(container.Id)
		jobs <- &Job{
			Data:   c,
			Name:   fmt.Sprintf("container:%s", container.Id),
			Encode: true,
		}
	}
}

func (s *Server) pushImages(jobs chan *Job, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	images := s.getImages()
	jobs <- &Job{
		Data:   images,
		Name:   fmt.Sprintf("images:%s", s.RaftServer.Name()),
		Encode: true,
	}
}

// Leader returns the current leader.
func (s *Server) Leader() string {
	l := s.RaftServer.Leader()
	if l == "" {
		// single node ; i am leader
		return s.RaftServer.Name()
	}
	return l
}

func (s *Server) GetConnectionString(node string) string {
	// self
	if node == s.RaftServer.Name() {
		return s.ConnectionString()
	}
	// master
	if node == s.RaftServer.Leader() {
		if s.LeaderURL == "" {
			return s.ConnectionString()
		} else {
			return fmt.Sprintf("http://%s", s.LeaderURL)
		}
	}
	// check peers
	for _, p := range s.RaftServer.Peers() {
		log.Printf("Peer: %s", p.ConnectionString)
		if p.Name == node {
			return p.ConnectionString
		}
	}
	return ""
}

// returns if this is the leader
func (s *Server) IsLeader() bool {
	return s.RaftServer.State() == raft.Leader
}

// returns the current members.
func (s *Server) Members() (members []string) {
	peers := s.RaftServer.Peers()

	for _, p := range peers {
		members = append(members, strings.TrimPrefix(p.ConnectionString, "http://"))
	}
	return
}

// returns all nodes in the cluster
func (s *Server) AllNodes() []string {
	var allHosts []string
	allHosts = append(allHosts, s.RaftServer.Name())
	for _, p := range s.RaftServer.Peers() {
		allHosts = append(allHosts, p.Name)
	}
	return allHosts
}

// redirects requests to the cluster leader
func (s *Server) redirectToLeader(w http.ResponseWriter, req *http.Request) {
	log.Printf("Redirecting %s", req.URL.Path)
	if s.Leader() != "" {
		leader := s.GetConnectionString(s.Leader())
		newPath := fmt.Sprintf("%s%s", leader, req.URL.Path)
		http.Redirect(w, req, newPath, http.StatusFound)
	} else {
		log.Println("Error: Leader Unknown")
		http.Error(w, "Leader unknown", http.StatusInternalServerError)
	}
}

// Returns the connection string.
func (s *Server) ConnectionString() string {
	return fmt.Sprintf("http://%s:%d", s.Host, s.Port)
}

// Starts the server.
func (s *Server) ListenAndServe(leader string) error {
	var err error

	log.Printf("Initializing Raft Server: %s", s.path)

	// Initialize and start Raft server.
	transporter := raft.NewHTTPTransporter("/raft")
	s.RaftServer, err = raft.NewServer(s.name, s.path, transporter, nil, s.db, "")
	if err != nil {
		log.Fatal(err)
	}
	transporter.Install(s.RaftServer, s)
	s.RaftServer.Start()

	if leader != "" {
		// Join to leader if specified.
		log.Println("Attempting to join leader:", leader)

		if !s.RaftServer.IsLogEmpty() {
			log.Fatal("Cannot join with an existing log")
		}
		if err := s.Join(leader); err != nil {
			log.Fatal(err)
		}

	} else if s.RaftServer.IsLogEmpty() {
		// Initialize the server by joining itself.

		log.Println("Initializing new cluster")

		_, err := s.RaftServer.Do(&raft.DefaultJoinCommand{
			Name:             s.RaftServer.Name(),
			ConnectionString: s.ConnectionString(),
		})
		if err != nil {
			log.Fatal(err)
		}

	} else {
		log.Println("Recovered from log")
	}

	log.Println("Initializing HTTP API")

	// Initialize and start HTTP server.
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.Port),
		Handler: s.Router,
	}

	s.Router.HandleFunc("/db/{key}", s.readHandler).Methods("GET").Name("db")
	s.Router.HandleFunc("/db/{key}", s.writeHandler).Methods("POST")
	s.Router.HandleFunc("/join", s.joinHandler).Methods("POST")
	s.Router.HandleFunc("/sync", s.syncHandler).Methods("GET").Name("sync")
	s.Router.HandleFunc("/docker/{path:.*}", s.dockerHandler).Methods("GET", "POST").Name("docker")
	//s.Router.HandleFunc("/{apiVersion:.*}/containers/json", s.containersHandler).Methods("GET", "POST")
	//s.Router.HandleFunc("/{apiVersion:.*}/images/json", s.imagesHandler).Methods("GET", "POST")
	//s.Router.HandleFunc("/{apiVersion:.*}/containers/{containerId:.*}/restart", s.containerRestartHandler).Methods("GET", "POST")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/json", s.containersHandler).Methods("GET", "POST")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/images/json", s.imagesHandler).Methods("GET", "POST")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/{containerId:.*}/restart", s.containerRestartHandler).Methods("GET", "POST")
	s.Router.HandleFunc("/", s.indexHandler).Methods("GET")

	log.Printf("Server name: %s\n", s.RaftServer.Name())
	log.Println("Listening at:", s.ConnectionString())

	// start sync
	go s.syncDocker(1)

	return s.httpServer.ListenAndServe()
}

// Gorilla mux not providing the correct net/http HandleFunc() interface.
func (s *Server) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	s.Router.HandleFunc(pattern, handler)
}

// Index handler
func (s *Server) indexHandler(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("Docker Cluster"))
}

// Joins to the leader of an existing cluster.
func (s *Server) Join(leader string) error {
	command := &raft.DefaultJoinCommand{
		Name:             s.RaftServer.Name(),
		ConnectionString: s.ConnectionString(),
	}

	var b bytes.Buffer
	json.NewEncoder(&b).Encode(command)
	resp, err := http.Post(fmt.Sprintf("http://%s/join", leader), "application/json", &b)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *Server) joinHandler(w http.ResponseWriter, req *http.Request) {
	command := &raft.DefaultJoinCommand{}

	if err := json.NewDecoder(req.Body).Decode(&command); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := s.RaftServer.Do(command); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) Sync() error {
	n := s.AllNodes()
	var nodes []string
	for _, v := range n {
		nodes = append(nodes, s.GetConnectionString(v))
	}
	command := NewSyncCommand(nodes)
	if _, err := s.RaftServer.Do(command); err != nil {
		return err
	}
	return nil
}

func (s *Server) syncHandler(w http.ResponseWriter, req *http.Request) {
	if err := s.Sync(); err != nil {
		switch err {
		case raft.NotLeaderError:
			s.redirectToLeader(w, req)
		default:
			log.Println("Error: ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handles reads from the db
func (s *Server) readHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	value := s.db.Get(vars["key"])
	w.Write([]byte(value))
}

// handles writes to the db
func (s *Server) writeHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	// Read the value from the POST body.
	b, err := ioutil.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	value := string(b)
	// Execute the command against the Raft server.
	if _, err := s.RaftServer.Do(NewWriteCommand(vars["key"], value)); err != nil {
		switch err {
		case raft.NotLeaderError:
			// re-post to leader
			host := s.GetConnectionString(s.Leader())
			url := fmt.Sprintf("%s%s", host, req.URL.Path)
			buf := bytes.NewBufferString(value)
			res, err := http.Post(url, "text/plain", buf)
			if err != nil {
				log.Printf("Error posting to the API: %s\n", err)
				return
			}
			res.Body.Close()
		default:
			log.Println("Error: ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) containersHandler(w http.ResponseWriter, req *http.Request) {
	all := req.FormValue("all")
	containersResponse(s, w, all)
}

func (s *Server) imagesHandler(w http.ResponseWriter, req *http.Request) {
	imageActionResponse(s, w)
}

func (s *Server) containerRestartHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	req.ParseForm()
	// Read the value from the POST body.
	b, err := ioutil.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	value := string(b)
	command := NewContainerRestartCommand(vars["containerId"], vars["apiVersion"], req.URL.Path, req.Form, s)
	if _, err := s.RaftServer.Do(command); err != nil {
		switch err {
		case raft.NotLeaderError:
			// re-post to leader
			host := s.GetConnectionString(s.Leader())
			url := fmt.Sprintf("%s%s", host, req.URL.Path)
			buf := bytes.NewBufferString(value)
			res, err := http.Post(url, "text/plain", buf)
			if err != nil {
				log.Printf("Error redirecting to the leader for containerRestart: %s\n", err)
				return
			}
			res.Body.Close()
		default:
			log.Println("Error: ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handlerError(msg string, status int, w http.ResponseWriter) {
	w.WriteHeader(status)
	w.Write([]byte(msg))
}

func (s *Server) dockerHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	path := fmt.Sprintf("/%s", strings.Replace(vars["path"], "docker", "", 1))
	log.Printf("Received Docker request: %s", path)
	c, err := s.newDockerClient()
	defer c.Close()
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	r, err := http.NewRequest(req.Method, path, nil)
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	resp, err := c.Do(r)
	defer resp.Body.Close()
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	w.Write([]byte(contents))
}
