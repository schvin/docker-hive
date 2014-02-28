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
	"fmt"
	"github.com/ehazlett/docker-hive/db"
	"github.com/goraft/raft"
	"log"
	"net/http"
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
func (c *WriteCommand) CommandName() string { return "db:write" }

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
func (c *ActionCommand) CommandName() string { return "hive:action" }

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
func (c *SyncCommand) CommandName() string { return "hive:sync" }

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
