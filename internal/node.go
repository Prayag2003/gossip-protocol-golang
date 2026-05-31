package internal

import "time"

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
	SuspectedAt  time.Time
}

func NewNode(id, addr string) Node {
	return Node{
		Id:           id,
		Addr:         addr,
		Status:       StatusAlive,
		HeartBeatSeq: uint64(time.Now().Unix()),
		LastSeen:     time.Time{},
	}
}
