package server

import (
	"github.com/goraft/raft"
	"github.com/ehazlett/docker-cluster/db"
	"log"
)

// -- write command
type WriteCommand struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Creates a new write command.
func NewWriteCommand(key string, value string) *WriteCommand {
	return &WriteCommand{
		Key:   key,
		Value: value,
	}
}

// The name of the command in the log.
func (c *WriteCommand) CommandName() string {
	return "write"
}

// Writes a value to a key.
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

// Creates a new write command.
func NewActionCommand(node string, action string) *ActionCommand {
	return &ActionCommand{
		Node:   node,
		Action: action,
	}
}

func (c *ActionCommand) CommandName() string {
	return "action"
}

// do action
func (c *ActionCommand) Apply(server raft.Server) (interface{}, error) {
	log.Printf("action for %s: %s\n", c.Node, c.Action)
	return nil, nil
}
