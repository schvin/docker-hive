package server

import (
	"fmt"
	"github.com/ehazlett/docker-cluster/db"
	"github.com/goraft/raft"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type UpdateJob struct {
	Path string
}

// -- write command
type WriteCommand struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// new write command
func NewWriteCommand(key string, value string) *WriteCommand {
	return &WriteCommand{
		Key:   key,
		Value: value,
	}
}

// name for log
func (c *WriteCommand) CommandName() string {
	return "write"
}

// writes a value to a key
func (c *WriteCommand) Apply(server raft.Server) (interface{}, error) {
	db := server.Context().(*db.DB)
	db.Put(c.Key, c.Value)
	return nil, nil
}

// -- action command
type ActionCommand struct {
	Node   string `json:"node"`
	Action string `json:"action"`
}

// new action command
func NewActionCommand(node string, action string) *ActionCommand {
	return &ActionCommand{
		Node:   node,
		Action: action,
	}
}

// name for log
func (c *ActionCommand) CommandName() string {
	return "action"
}

// do action
func (c *ActionCommand) Apply(server raft.Server) (interface{}, error) {
	log.Printf("action for %s: %s\n", c.Node, c.Action)
	return nil, nil
}

// -- sync command
type SyncCommand struct {
	Nodes []string `json:"nodes"`
}

// sync command
func NewSyncCommand(nodes []string) *SyncCommand {
	return &SyncCommand{
		Nodes: nodes,
	}
}

// name for log
func (c *SyncCommand) CommandName() string {
	return "sync"
}
func update(jobs <-chan *UpdateJob, group *sync.WaitGroup) {
	group.Add(1)
	defer group.Done()
	for j := range jobs {
		http.Get(j.Path)
	}
}

func (c *SyncCommand) Apply(server raft.Server) (interface{}, error) {
	syncGroup := &sync.WaitGroup{}
	var jobs = make(chan *UpdateJob, len(c.Nodes))
	go update(jobs, syncGroup)
	for _, v := range c.Nodes {
		jobs <- &UpdateJob{
			Path: fmt.Sprintf("%s/update", v),
		}
	}
	syncGroup.Wait()
	return nil, nil
}

// -- restart container command
type ContainerRestartCommand struct {
	ContainerId string     `json:"container_id"`
	ApiVersion  string     `json:"api_version"`
	Path        string     `json:"path"`
	Params      url.Values `json:"params"`
	Server      *Server
}

// container restart command
func NewContainerRestartCommand(containerId string, apiVersion string, path string, params url.Values, server *Server) *ContainerRestartCommand {
	return &ContainerRestartCommand{
		ContainerId: containerId,
		ApiVersion:  apiVersion,
		Path:        path,
		Params:      params,
		Server:      server,
	}
}

// name for log
func (c *ContainerRestartCommand) CommandName() string {
	return "containerRestart"
}

func (c *ContainerRestartCommand) Apply(server raft.Server) (interface{}, error) {
	db := server.Context().(*db.DB)
	// look for container
	key := fmt.Sprintf("container:host:%s", c.ContainerId)
	hostInfo := db.Find(key)
	if hostInfo == "" {
		log.Fatalf("Unable to find Docker host for %s", c.ContainerId)
	}
	parts := strings.Split(hostInfo, "::")
	serverName := parts[0]
	host := parts[1]
	if serverName == server.Name() {
		log.Printf("Restarting container %s on %s", c.ContainerId, serverName)
		path := fmt.Sprintf("%s/docker%s?%s", host, c.Path, c.Params.Encode())
		log.Printf("ContainerRestartCommand.Apply %s", path)
		//dockerAPIrequest(path, "POST", c.Server)
		log.Printf("Docker API Request: %s", path)
		client := &http.Client{}
		req, err := http.NewRequest("POST", path, nil)
		if err != nil {
			log.Fatalf("Error communicating with Docker: %s", err)
			return nil, err
		}
		// send to docker
		go client.Do(req)
	}
	return nil, nil
}
