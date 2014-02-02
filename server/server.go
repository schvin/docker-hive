package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dotcloud/docker"
	"github.com/goraft/raft"
	"github.com/gorilla/mux"
	"github.com/ehazlett/docker-cluster/db"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

type Server struct {
	name       string
	host       string
	port       int
	path       string
	httpServer *http.Server
	db         *db.DB
	mutex      sync.RWMutex
	Router     *mux.Router
	RaftServer raft.Server
}

// Creates a new server.
func New(path string, host string, port int) *Server {
	s := &Server{
		host:   host,
		port:   port,
		path:   path,
		db:     db.New(),
		Router: mux.NewRouter(),
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
	for _, p := range s.RaftServer.Peers() {
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

// redirects requests to the cluster leader
func (s *Server) redirectToLeader(w http.ResponseWriter, req *http.Request) {
	if s.Leader() != "" {
		leader := s.GetConnectionString(s.Leader())
		http.Redirect(w, req, leader+req.URL.Path, http.StatusMovedPermanently)
	} else {
		log.Println("Error: Leader Unknown")
		http.Error(w, "Leader unknown", http.StatusInternalServerError)
	}
}

// Returns the connection string.
func (s *Server) ConnectionString() string {
	return fmt.Sprintf("http://%s:%d", s.host, s.port)
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
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.Router,
	}

	s.Router.HandleFunc("/db/{key}", s.readHandler).Methods("GET").Name("db")
	s.Router.HandleFunc("/db/{key}", s.writeHandler).Methods("POST")
	s.Router.HandleFunc("/join", s.joinHandler).Methods("POST")
	s.Router.HandleFunc("/{apiVersion:.*}/{action:.*}/{format:.*}", s.actionHandler).Methods("GET", "POST")
	s.Router.HandleFunc("/", s.indexHandler).Methods("GET")

	log.Printf("Server name: %s\n", s.RaftServer.Name())
	log.Println("Listening at:", s.ConnectionString())

	return s.httpServer.ListenAndServe()
}

// Gorilla mux not providing the correct net/http HandleFunc() interface.
func (s *Server) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	s.Router.HandleFunc(pattern, handler)
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

func (s *Server) readHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	value := s.db.Get(vars["key"])
	w.Write([]byte(value))
}

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
			s.redirectToLeader(w, req)
		default:
			log.Println("Error: ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) actionHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	switch vars["action"] {
	case "containers":
		all := req.FormValue("all")
		containerActionResponse(s, w, all)
		return
	}
	http.Error(w, "404", http.StatusNotFound)
}

func containerActionResponse(s *Server, w http.ResponseWriter, all string) {
	var allHosts []string
	allHosts = append(allHosts, s.RaftServer.Name())
	for _, p := range s.RaftServer.Peers() {
		allHosts = append(allHosts, p.Name)
	}
	value := "{}"
	var newContainers []*docker.APIContainers
	for _, host := range allHosts {
		key := fmt.Sprintf("containers:%s", host)
		value = s.db.Get(key)
		// filter out not running
		if all == "" && value != "" {
			var containers []*docker.APIContainers
			s := bytes.NewBufferString(value)
			d := json.NewDecoder(s)
			if err := d.Decode(&containers); err != nil {
				log.Printf("Error decoding container JSON: %s", err)
			}
			for _, v := range containers {
				if strings.Index(v.Status, "Up") != -1 {
					newContainers = append(newContainers, v)
				}
			}
		}
		b, err := json.Marshal(newContainers)
		if err != nil {
			log.Printf("Error marshaling containers to JSON: %s", err)
		}
		value = string(b)
	}
	w.Write([]byte(value))
}

func (s *Server) indexHandler(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("Docker Cluster"))
}
