package main

import "time"

type NodeState string

const (
	StatusAlive    NodeState = "alive"
	StatusSuspcted NodeState = "suspected"
	StatusDead     NodeState = "dead"
)

type Node struct {
	Id           string
	Addr         string
	State        NodeState
	HeartBeatSeq uint64
	LastSeen     time.Time
}

func main() {

}
