package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dotcloud/docker"
	"github.com/ehazlett/docker-hive/db"
	"github.com/goraft/raft"
	"github.com/gorilla/mux"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"strings"
	"sync"
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

func copyHeaders(dst, src http.Header) {
	// copy headers
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

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

func (s *Server) AllNodeConnectionStrings() []string {
	var allHosts []string
	allHosts = append(allHosts, s.ConnectionString())
	for _, p := range s.RaftServer.Peers() {
		allHosts = append(allHosts, s.GetConnectionString(p.Name))
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
	s.Router.HandleFunc("/docker/{path:.*}", s.dockerHandler).Methods("GET", "POST", "DELETE").Name("docker")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/json", s.containersHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/create", s.containerCreateHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/{containerId:.*}/json", s.containerInspectHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/{containerId:.*}", s.containerRemoveHandler).Methods("DELETE")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/{containerId:.*}/restart", s.containerRestartHandler).Methods("GET", "POST")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/{containerId:.*}/start", s.containerStartHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/{containerId:.*}/stop", s.containerStopHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/{containerId:.*}/top", s.containerTopHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/{containerId:.*}/changes", s.containerChangesHandler).Methods("GET")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/containers/{containerId:.*}/kill", s.containerKillHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/images/json", s.imagesHandler).Methods("GET", "POST")
	s.Router.HandleFunc("/{apiVersion:v1.[7-9]}/images/create", s.imageCreateHandler).Methods("POST")
	s.Router.HandleFunc("/", s.indexHandler).Methods("GET")

	log.Printf("Server name: %s\n", s.RaftServer.Name())
	log.Println("Listening at:", s.ConnectionString())

	return s.httpServer.ListenAndServe()
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
	imagesResponse(s, w)
}

func (s *Server) imageCreateHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

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
	// Read the value from the POST body.
	//_, err := ioutil.ReadAll(req.Body)
	//if err != nil {
	//	w.WriteHeader(http.StatusBadRequest)
	//	return
	//}

	for _, host := range s.AllNodeConnectionStrings() {
		params := req.Form
		urlString := fmt.Sprintf("%s/docker%s?%s", host, req.URL.Path, params.Encode())
		s.proxyRequest(w, req, urlString)
	}
}

func (s *Server) containerCreateHandler(w http.ResponseWriter, req *http.Request) {
	params := req.Form
	n := req.FormValue("node")
	host := s.GetConnectionString(n)
	if n == "" {
		n = s.RaftServer.Name()
		host = s.ConnectionString()
	}
	log.Printf("Launching container on %s", n)
	urlString := fmt.Sprintf("%s/docker%s?%s", host, req.URL.Path, params.Encode())
	s.proxyRequest(w, req, urlString)
}

func (s *Server) containerRestartHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

func (s *Server) containerStartHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

func (s *Server) containerStopHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

func (s *Server) containerRemoveHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

func (s *Server) containerTopHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

func (s *Server) containerChangesHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}

func (s *Server) containerKillHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
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

func (s *Server) containerInspectHandler(w http.ResponseWriter, req *http.Request) {
	s.proxyDockerRequest(w, req)
}