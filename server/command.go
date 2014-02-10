package server

import (
	"fmt"
	"github.com/ehazlett/docker-cluster/db"
	"github.com/goraft/raft"
	"log"
	"net/http"
	"net/url"
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
                Server:     server,
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
	host := db.Find(key)
        if host == server.Name() {
	    log.Printf("Restarting container %s", c.ContainerId)
            h := fmt.Sprintf("http://%s:%d", c.Server.Host, c.Server.Port)
            path := fmt.Sprintf("%s/docker%s?%s", h, c.Path, c.Params.Encode())
            // TODO: finish container restart
            log.Printf("ContainerRestartCommand.Apply %s", path)
        }
	return nil, nil
}
