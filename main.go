package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusAlive    Status = "alive"
	StatusSuspcted Status = "suspected"
	StatusDead     Status = "dead"
)

type Node struct {
	Id           string
	Addr         string
	Status       Status
	HeartBeatSeq uint64
	LastSeen     time.Time
}

type KVEntry struct {
	Value string
	// logical clock, higher wins
	Version uint64
}

type LocalNode struct {
	Self    Node
	Members map[string]Node
	KVStore map[string]KVEntry
	mu      sync.RWMutex
}

type GossipMessage struct {
	From    Node
	Members map[string]Node
	KV      map[string]KVEntry
}

func NewNode(id, addr string) Node {
	return Node{
		Id:           id,
		Addr:         addr,
		Status:       StatusAlive,
		HeartBeatSeq: 0,
		LastSeen:     time.Time{},
	}
}

func NewLocalNode(id, addr string) *LocalNode {

	self := NewNode(id, addr)

	members := make(map[string]Node)
	members[self.Id] = self

	return &LocalNode{
		Self:    self,
		Members: members,
		KVStore: map[string]KVEntry{},
	}
}

func (n *LocalNode) String() string {
	var members strings.Builder
	for id, node := range n.Members {
		fmt.Fprintf(
			&members,
			"\n    %s => {status=%s hb=%d}",
			id,
			node.Status,
			node.HeartBeatSeq,
		)
	}

	var kvs strings.Builder
	for key, entry := range n.KVStore {
		fmt.Fprintf(
			&kvs,
			"\n    %s => {value=%q version=%d}",
			key,
			entry.Value,
			entry.Version,
		)
	}

	return fmt.Sprintf(
		`Node=%s
Members (%d):%s
KV Entries (%d):%s`,
		n.Self.Id,
		len(n.Members),
		members.String(),
		len(n.KVStore),
		kvs.String(),
	)
}

// func (n *LocalNode) PrettyPrint() {
// 	b, err := json.MarshalIndent(n, "", " ")
// 	if err != nil {
// 		fmt.Println(err)
// 		return
// 	}
// 	fmt.Println(string(b))
// }

func (ln *LocalNode) merge(msg GossipMessage) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	// For Heartbeat
	for key, incomingNode := range msg.Members {
		if key == ln.Self.Id {
			continue
		}

		existing, ok := ln.Members[key]
		if !ok {
			ln.Members[key] = incomingNode
		} else if incomingNode.HeartBeatSeq > existing.HeartBeatSeq {
			ln.Members[key] = incomingNode
		}
	}

	// For KV Pairs
	for key, incomingVal := range msg.KV {
		existing, ok := ln.KVStore[key]
		if !ok {
			ln.KVStore[key] = incomingVal
		} else if incomingVal.Version > existing.Version {
			ln.KVStore[key] = incomingVal
		}
	}
}

func main() {
	a := NewLocalNode("node-A", ":8000")
	a.Members["node-B"] = Node{Id: "node-B", Addr: ":8081", HeartBeatSeq: 2}
	a.KVStore["leader"] = KVEntry{Value: "node-A", Version: 1}

	// a.PrettyPrint()
	fmt.Println(a.String())

	incoming := GossipMessage{
		From: NewNode("node-B", ":8081"),
		Members: map[string]Node{
			"node-A": {Id: "node-A", HeartBeatSeq: 99},
			"node-B": {Id: "node-B", HeartBeatSeq: 10},
			"node-C": {Id: "node-C", HeartBeatSeq: 1},
		},
		KV: map[string]KVEntry{
			"leader":  {Value: "node-B", Version: 5},
			"replica": {Value: "2", Version: 1},
		},
	}

	a.merge(incoming)

	fmt.Println("=== Members ===")
	for id, node := range a.Members {
		fmt.Printf("  %s → seq=%d\n", id, node.HeartBeatSeq)
	}

	fmt.Println("=== KV ===")
	for key, entry := range a.KVStore {
		fmt.Printf("  %s → value=%s version=%d\n", key, entry.Value, entry.Version)
	}
}
