package main

import (
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
	State        Status
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

func main() {

}
