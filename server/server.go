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
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ehazlett/docker-hive/db"
	"github.com/goraft/raft"
	"github.com/gorilla/mux"
)

type (
	Job struct {
		Name   string
		Data   interface{}
		Encode bool
	}

	Image struct {
		Id          string
		Created     int64
		RepoTags    []string
		Size        int
		VirtualSize int
	}

	InfoPort struct {
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
		Ports   []InfoPort
		Names   []string
	}

	Server struct {
		name        string
		Host        string
		Port        int
		path        string
		httpServer  *http.Server
		db          *db.DB
		mutex       sync.RWMutex
		waiter      *sync.WaitGroup
		Router      *mux.Router
		RaftServer  raft.Server
		DockerPath  string
		LeaderURL   string
		peerTimeout int
		Version     string
	}

	ServerInfo struct {
		Name             string   `json:"name"`
		Port             int      `json:"port"`
		ConnectionString string   `json:"connectionString"`
		Version          string   `json:"version"`
		DockerPath       string   `json:"dockerPath"`
		IsLeader         bool     `json:"isLeader"`
		Leader           string   `json:"leader"`
		Peers            []string `json:"peers"`
	}

	ContainerInfo struct {
		Container  Container
		ServerName string
	}

	ImageDetail struct {
		Id            string
		Name          string
		Created       time.Time
		DockerVersion string `json:"docker_version"`
		Size          int64
		Architecture  string
		Author        string
	}

	ImageInfo struct {
		Image      ImageDetail
		ServerName string
	}

	ServerContext struct {
		Server *Server
	}
)

// Utility function for copying HTTP Headers.
func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// Creates a new server.
func New(path string, host string, port int, dockerPath string, leader string, peerTimeout int, version string) *Server {
	s := &Server{
		path:        path,
		Host:        host,
		Port:        port,
		DockerPath:  dockerPath,
		LeaderURL:   leader,
		peerTimeout: peerTimeout,
		db:          db.New(),
		waiter:      new(sync.WaitGroup),
		Router:      mux.NewRouter(),
		Version:     version,
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

// Creates a new Docker client using the Docker unix socket.
func (s *Server) newDockerClient() (*httputil.ClientConn, error) {
	conn, err := net.Dial("unix", s.DockerPath)
	if err != nil {
		return nil, err
	}
	return httputil.NewClientConn(conn, nil), nil
}

// Gets all containers among the cluster with the specified id.
func (s *Server) getContainerInfo(id string) []ContainerInfo {
	found := false
	var containers []ContainerInfo
	for _, serverName := range s.AllNodes() {
		if found {
			break
		}
		host := s.GetConnectionString(serverName)
		path := fmt.Sprintf("%s/docker/containers/%s/json", host, id)
		resp, err := http.Get(path)
		if err != nil {
			log.Printf("Error getting host containers for %s: %s", host, err)
			continue
		}
		// if container doesn't exist on host, skip
		if resp.StatusCode == 404 {
			continue
		}
		contents, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		// filter out not running
		var container Container
		c := bytes.NewBufferString(string(contents))
		d := json.NewDecoder(c)
		if err := d.Decode(&container); err != nil {
			log.Printf("Error decoding container JSON: %s", err)
			continue
		}
		// search for the container id (in case "short form" is sent
		if strings.IndexAny(container.Id, id) != -1 {
			containerInfo := ContainerInfo{Container: container, ServerName: serverName}
			containers = append(containers, containerInfo)
		}
	}
	return containers
}

// Gets all images among the cluster with the specified id.
func (s *Server) getImageInfo(id string) []ImageInfo {
	found := false
	var images []ImageInfo
	for _, serverName := range s.AllNodes() {
		if found {
			break
		}
		host := s.GetConnectionString(serverName)
		path := fmt.Sprintf("%s/docker/images/%s/json", host, id)
		resp, err := http.Get(path)
		if err != nil {
			log.Printf("Error getting host images for %s: %s", host, err)
			continue
		}
		// if container doesn't exist on host, skip
		if resp.StatusCode == 404 {
			continue
		}
		contents, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		// filter out not running
		var img ImageDetail
		i := bytes.NewBufferString(string(contents))
		d := json.NewDecoder(i)
		if err := d.Decode(&img); err != nil {
			log.Printf("Error decoding image JSON: %s", err)
			continue
		}
		img.Name = id
		// search for the container id (in case "short form" is sent
		if strings.IndexAny(img.Id, id) != -1 {
			imageInfo := ImageInfo{Image: img, ServerName: serverName}
			images = append(images, imageInfo)
		}
	}
	return images
}

// Utility function for getting all local Docker containers.
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

// Utility function for inspecting a local Docker container.
func (s *Server) inspectContainer(id string) *Container {
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

	var container *Container
	if resp.StatusCode == http.StatusOK {
		d := json.NewDecoder(resp.Body)
		if err = d.Decode(&container); err != nil {
			log.Fatalf("Erroring decoding container JSON: %s", err)
		}
	}
	resp.Body.Close()
	return container
}

// Utility function for getting local Docker images.
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

// Leader returns the current leader.
func (s *Server) Leader() string {
	l := s.RaftServer.Leader()
	if l == "" {
		// single node ; i am leader
		return s.RaftServer.Name()
	}
	return l
}

// This returns the connection URL for the specified node.
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
		if p.Name == node {
			return p.ConnectionString
		}
	}
	return ""
}

// This returns if this is the leader.
func (s *Server) IsLeader() bool {
	return s.RaftServer.State() == raft.Leader
}

func (s *Server) RemovePeer(name string) error {
	// Remove peer in raft
	err := s.RaftServer.RemovePeer(name)
	if err != nil {
		log.Printf("Unable to remove peer: %s (%v)", name, err)
		return err
	}
	return nil
}

// This returns the current members.
func (s *Server) Members() (members []string) {
	peers := s.RaftServer.Peers()

	for _, p := range peers {
		members = append(members, strings.TrimPrefix(p.ConnectionString, "http://"))
	}
	return
}

// This returns all nodes in the cluster.
func (s *Server) AllNodes() []string {
	var allHosts []string
	allHosts = append(allHosts, s.RaftServer.Name())
	for _, p := range s.RaftServer.Peers() {
		allHosts = append(allHosts, p.Name)
	}
	return allHosts
}

// This returns connection strings for all nodes in the cluster.
func (s *Server) AllNodeConnectionStrings() []string {
	var allHosts []string
	allHosts = append(allHosts, s.ConnectionString())
	if len(s.RaftServer.Peers()) > 0 {
		for _, p := range s.RaftServer.Peers() {
			allHosts = append(allHosts, s.GetConnectionString(p.Name))
		}
	}
	return allHosts
}

// Redirects requests to the cluster leader.
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
func (s *Server) Start() (*sync.WaitGroup, error) {
	var err error

	log.Printf("Initializing Raft Server: %s", s.path)

	// Initialize and start Raft server.
	transporter := raft.NewHTTPTransporter("/raft")
	ctx := ServerContext{Server: s}
	s.RaftServer, err = raft.NewServer(s.name, s.path, transporter, nil, ctx, "")
	if err != nil {
		log.Fatal(err)
	}
	transporter.Install(s.RaftServer, s)
	s.RaftServer.Start()
	leader := s.LeaderURL
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
	s.Router.HandleFunc("/info", s.infoHandler).Methods("GET").Name("info")
	s.Router.HandleFunc("/docker/{path:.*}", s.dockerHandler).Methods("GET", "POST", "DELETE").Name("docker")
	s.Router.HandleFunc("/{apiVersion:v1.*}/auth", s.dockerAuthHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.*}/version", s.dockerVersionHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.*}/info", s.dockerInfoHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/json", s.containersHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/create", s.containerCreateHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/{containerId:.*}/json", s.containerInspectHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/{containerId:.*}", s.containerRemoveHandler).Methods("DELETE")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/{containerId:.*}/restart", s.containerRestartHandler).Methods("GET", "POST")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/{containerId:.*}/start", s.containerStartHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/{containerId:.*}/stop", s.containerStopHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/{containerId:.*}/top", s.containerTopHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/{containerId:.*}/changes", s.containerChangesHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/{containerId:.*}/kill", s.containerKillHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.*}/containers/{containerId:.*}/export", s.containerExportHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.*}/images/json", s.imagesHandler).Methods("GET", "POST")
	s.Router.HandleFunc("/{apiVersion:v1.*}/images/create", s.imageCreateHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.*}/images/{imageName:.*}/history", s.imageHistoryHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.*}/images/{imageName:.*}/search", s.imageSearchHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.*}/images/{imageName:.*}/tag", s.imageSearchHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.*}/images/{imageName:.*}", s.imageDeleteHandler).Methods("DELETE")
	s.Router.HandleFunc("/", s.indexHandler).Methods("GET")

	log.Printf("Server name: %s\n", s.RaftServer.Name())
	log.Printf("Peer Timeout: %ds", s.peerTimeout)
	log.Println("Listening at:", s.ConnectionString())

	go s.listenAndServe()
	s.waiter.Add(1)
	go s.run()
	return s.waiter, nil
}

func (s *Server) Stop() {
	log.Println("Stopping server")
	s.waiter.Done()
}

func (s *Server) removeStalePeers() {
	if s.IsLeader() {
		for _, p := range s.RaftServer.Peers() {
			// ignore join events
			creationTime := time.Date(0001, time.January, 01, 0, 0, 0, 0, time.UTC)
			if p.LastActivity().Equal(creationTime) {
				continue
			}
			// if over the heartbeat interval, remove
			timeout, _ := time.ParseDuration(fmt.Sprintf("%ds", s.peerTimeout))
			drift := time.Now().Sub(p.LastActivity())
			if drift.Seconds() > timeout.Seconds() {
				log.Printf("Peer %s timeout; removing", p.Name)
				s.RemovePeer(p.Name)
			}
		}
	}
}

func (s *Server) listenAndServe() {
	go func() {
		s.httpServer.ListenAndServe()
	}()
}

func (s *Server) run() {
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)

	tick := time.Tick(1 * time.Second)

run:
	for {
		select {
		case <-tick:
			// check heartbeats of servers and remove if stale
			s.removeStalePeers()
		case <-sig:
			break run
		}
	}
	s.Stop()
}

// Gorilla mux not providing the correct net/http HandleFunc() interface.
func (s *Server) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	s.Router.HandleFunc(pattern, handler)
}

// Index handler
func (s *Server) indexHandler(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("Docker Hive"))
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

// Handles the join request.
func (s *Server) joinHandler(w http.ResponseWriter, req *http.Request) {
	command := &raft.DefaultJoinCommand{}

	if err := json.NewDecoder(req.Body).Decode(&command); err != nil {
		log.Printf("Error joining: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := s.RaftServer.Do(command); err != nil {
		log.Printf("Error joining: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("%s joined", req.RemoteAddr)
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

// Cluster Info
func (s *Server) infoHandler(w http.ResponseWriter, req *http.Request) {
	srv := ServerInfo{
		Name:             s.name,
		Port:             s.Port,
		ConnectionString: s.GetConnectionString(s.name),
		Version:          s.Version,
		DockerPath:       s.DockerPath,
		IsLeader:         s.IsLeader(),
		Leader:           s.GetConnectionString(s.Leader()),
		Peers:            s.Members(),
	}
	b, err := json.Marshal(srv)
	if err != nil {
		log.Printf("Error marshaling server info to JSON: %s", err)
	}
	value := string(b)
	w.Write([]byte(value))
}

// Docker: login
func (s *Server) dockerAuthHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyLocalDockerRequest(w, req)
}

// Docker: version
func (s *Server) dockerVersionHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: need to find a way to return multiple versions for each cluster node
	// for now will use local node
	s.proxyLocalDockerRequest(w, req)
}

// Docker: info
func (s *Server) dockerInfoHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: need to find a way to return multiple versions for each cluster node
	// for now will use local node
	s.proxyLocalDockerRequest(w, req)
}

// Docker: list containers
func (s *Server) containersHandler(w http.ResponseWriter, req *http.Request) {
	all := req.FormValue("all")
	containersResponse(s, w, all)
}

// Docker: list images
func (s *Server) imagesHandler(w http.ResponseWriter, req *http.Request) {
	imagesResponse(s, w)
}

// Docker: pull image
func (s *Server) imageCreateHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: image history
func (s *Server) imageHistoryHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: image search
func (s *Server) imageSearchHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: image tag
func (s *Server) imageTagHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: delete image
func (s *Server) imageDeleteHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Proxies HTTP requests
func (s *Server) proxyRequest(w http.ResponseWriter, req *http.Request, urlString string) {
	client := &http.Client{}
	if urlString == "" {
		urlString = req.URL.String()
	}
	r, err := http.NewRequest(req.Method, urlString, req.Body)
	// copy headers
	copyHeaders(r.Header, req.Header)
	if err != nil {
		log.Fatalf("Error communicating with Docker: %s", err)
	}
	// send to docker
	resp, err := client.Do(r)
	if err != nil {
		log.Fatalf("Error communicating with Docker: %s", err)
	}
	contents, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Fatalf("Error response from Docker: %s", err)
	}
	w.WriteHeader(resp.StatusCode)
	w.Write([]byte(contents))
}

// Proxies request to local Docker instance.
func (s *Server) proxyLocalDockerRequest(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	params := req.Form
	path := fmt.Sprintf("%s?%s", req.URL.Path, params.Encode())
	log.Printf("Proxying Docker request: %s", path)
	c, err := s.newDockerClient()
	defer c.Close()
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	r, err := http.NewRequest(req.Method, path, req.Body)
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	resp, err := c.Do(r)
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	contents, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	w.WriteHeader(resp.StatusCode)
	io.WriteString(w, string(contents))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// Proxies requests to Docker specifically for Docker for all Nodes.
func (s *Server) proxyDockerRequest(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	servers := s.AllNodes()
	containerRequest := false
	imageRequest := false
	for k, _ := range vars {
		if k == "containerId" {
			containerRequest = true
		}

		if k == "imageId" {
			imageRequest = true
		}
	}
	// check if a container request and build server list with only those
	// that have the container present
	if containerRequest {
		servers = []string{}
		for _, c := range s.getContainerInfo(vars["containerId"]) {
			servers = append(servers, c.ServerName)
		}
	}
	// check if an image request and build server list with only those
	// that have the image present
	if imageRequest {
		servers = []string{}
		for _, c := range s.getImageInfo(vars["imageId"]) {
			servers = append(servers, c.ServerName)
		}
	}
	req.ParseForm()
	params := req.Form
	for _, srv := range servers {
		host := s.GetConnectionString(srv)
		urlString := fmt.Sprintf("%s/docker%s?%s", host, req.URL.Path, params.Encode())
		s.proxyRequest(w, req, urlString)
		// TODO: for now we must break because the Docker CLI
		// cannot handle multiple responses however we
		// want the scaffolding in here for when we get the hive cli
		// and/or Docker CLI integration
		break
	}
}

// Docker: run
func (s *Server) containerCreateHandler(w http.ResponseWriter, req *http.Request) {
	params := req.Form
	target := req.FormValue("target")
	n := ""
	host := ""
	// default to random node
	if target == "" {
		n = s.randomNode()
		host = s.GetConnectionString(n)
	}
	log.Printf("Using host %s for container", host)
	log.Printf("Launching container on %s", n)
	urlString := fmt.Sprintf("%s/docker%s?%s", host, req.URL.Path, params.Encode())
	s.proxyRequest(w, req, urlString)
}

// Docker: inspect
func (s *Server) containerInspectHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: restart
func (s *Server) containerRestartHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: start
func (s *Server) containerStartHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: stop
func (s *Server) containerStopHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: rm
func (s *Server) containerRemoveHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: top
func (s *Server) containerTopHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: diff
func (s *Server) containerChangesHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: kill
func (s *Server) containerKillHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

// Docker: export
func (s *Server) containerExportHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: need to find a way to return multiple versions for each cluster node
	// for now will use first node on which the container is found
	vars := mux.Vars(req)
	for _, c := range s.getContainerInfo(vars["containerId"]) {
		tUrl, _ := url.Parse(s.GetConnectionString(c.ServerName))
		path := fmt.Sprintf("/docker%s", req.URL.Path)
		req.URL.Host = tUrl.Host
		req.URL.Scheme = tUrl.Scheme
		req.URL.Path = path
		s.proxyRequest(w, req, req.URL.String())
		break
	}
}

// Generic error handler.
func handlerError(msg string, status int, w http.ResponseWriter) {
	w.WriteHeader(status)
	w.Write([]byte(msg))
}

// Proxies requests to the local Docker daemon
func (s *Server) dockerHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	req.ParseForm()
	params := req.Form
	path := fmt.Sprintf("/%s?%s", strings.Replace(vars["path"], "docker", "", 1), params.Encode())
	log.Printf("Received Docker request: %s", path)
	c, err := s.newDockerClient()
	defer c.Close()
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	r, err := http.NewRequest(req.Method, path, req.Body)
	copyHeaders(r.Header, req.Header)
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	resp, err := c.Do(r)
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	contents, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		msg := fmt.Sprintf("Error connecting to Docker: %s", err)
		log.Println(msg)
		handlerError(msg, http.StatusInternalServerError, w)
		return
	}
	w.WriteHeader(resp.StatusCode)
	w.Write([]byte(contents))
}
