package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"maps"
	"math/rand"
	"net/http"
	"os"
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
	SuspectedAt  time.Time
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

	slog.Info("Created a node, Port: ", "addr", addr)
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
			slog.Info("Discovered new member", "id", key, "addr", incomingNode.Addr)
		} else if incomingNode.HeartBeatSeq > existing.HeartBeatSeq {
			ln.Members[key] = incomingNode
		} else if existing.Status != StatusAlive && incomingNode.Status == StatusAlive {
			// resurrection
			ln.Members[key] = incomingNode
			slog.Info("Node Resurrected", "id", key)
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

func (ln *LocalNode) buildGossipMessage() GossipMessage {
	ln.mu.RLock()
	defer ln.mu.RUnlock()

	members := make(map[string]Node, len(ln.Members))
	kv := make(map[string]KVEntry, len(ln.KVStore))

	maps.Copy(members, ln.Members)
	maps.Copy(kv, ln.KVStore)

	return GossipMessage{
		From:    ln.Self,
		Members: members,
		KV:      kv,
	}

}

func (ln *LocalNode) gossipHandler(w http.ResponseWriter, r *http.Request) {
	// decode the r.body into a GossipMessage
	var gossipMessage GossipMessage

	const MB = 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, 1*MB)

	err := json.NewDecoder(r.Body).Decode(&gossipMessage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	slog.Info("Received gossip", "from", gossipMessage.From.Id, "members", len(gossipMessage.Members))
	ln.merge(gossipMessage)

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(ln.buildGossipMessage())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

var SECOND = time.Second

// I want to gossip to others
func (ln *LocalNode) pushPull(peer Node) error {
	myGossip := ln.buildGossipMessage()

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(myGossip); err != nil {
		return fmt.Errorf("failed to encode gossip payload: %s", err.Error())
	}

	// post as JSON to peer.Addr + /gossip
	url := "http://localhost" + peer.Addr + "/gossip"
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create post request")
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * SECOND,
	}

	resp, err := client.Do(req)
	if err != nil {
		ln.mu.Lock()
		if member, ok := ln.Members[peer.Id]; ok {
			member.Status = StatusSuspcted
			member.SuspectedAt = time.Now()
			ln.Members[peer.Id] = member
		}

		ln.mu.Unlock()
		return fmt.Errorf("POST request to %s failed: %s", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("POST error, statusCode expected: 200, received: %d", resp.StatusCode)
	}

	// decode the peer's response into gossipMessage
	var gossipMessage GossipMessage
	if err := json.NewDecoder(resp.Body).Decode(&gossipMessage); err != nil {
		return fmt.Errorf("error while decoding response body: %s", err.Error())
	}

	// call merge on the response
	slog.Info("pushPull complete", "peer", peer.Id, "theirMembers", len(gossipMessage.Members))
	ln.merge(gossipMessage)
	return nil
}

func (ln *LocalNode) getStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ln.buildGossipMessage())
}

func main() {
	id := os.Args[1]
	addr := os.Args[2]
	seed := os.Args[3]

	node := NewLocalNode(id, addr)

	if seed != "" {
		seedNode := NewNode("seed", seed)
		if err := node.pushPull(seedNode); err != nil {
			log.Printf("initial seed contact failed: %v", err)
		}
	}

	// Heartbeat Tick
	go func() {
		for range time.Tick(3 * SECOND) {
			node.mu.Lock()
			slog.Info("Incrementing Heartbeat")
			node.Self.HeartBeatSeq++
			node.Members[node.Self.Id] = node.Self
			node.mu.Unlock()
		}
	}()

	const K = 3

	go func() {
		for range time.Tick(10 * SECOND) {
			slog.Info("Starting gossip using ticks")
			node.mu.Lock()

			// collect non-self members into a slice
			var peers []Node
			for id, peer := range node.Members {
				if id != node.Self.Id && peer.Status == StatusAlive {
					peers = append(peers, peer)
				}

				switch peer.Status {
				case StatusSuspcted:
					if time.Since(peer.SuspectedAt) > 15*SECOND {
						peer.Status = StatusDead
						node.Members[id] = peer
						slog.Warn("Node declared as dead", "id", id)
					}

				case StatusDead:
					if time.Since(peer.SuspectedAt) > 60*SECOND {
						delete(node.Members, id)
						slog.Warn("Node removed from members", "id", id)
					}
				}
			}
			node.mu.Unlock()

			if len(peers) == 0 {
				continue
			}

			// randomly shuffle and pick min(K, len(peers))
			rand.Shuffle(len(peers), func(i, j int) {
				peers[i], peers[j] = peers[j], peers[i]
			})

			// call pushPull
			fanOut := min(K, len(peers))
			for _, peer := range peers[:fanOut] {

				go func(p Node) {
					if err := node.pushPull(p); err != nil {
						fmt.Println(fmt.Errorf("pushPull failed to %s: %v", p.Id, err.Error()))
					}
				}(peer)
			}

		}
	}()

	http.HandleFunc("/gossip", node.gossipHandler)
	http.HandleFunc("/status", node.getStatus)
	log.Fatal(http.ListenAndServe(addr, nil))

	// a.PrettyPrint()
	// fmt.Println(a.String())

	// incoming := GossipMessage{
	// 	From: NewNode("node-B", ":8081"),
	// 	Members: map[string]Node{
	// 		"node-A": {Id: "node-A", HeartBeatSeq: 99},
	// 		"node-B": {Id: "node-B", HeartBeatSeq: 10},
	// 		"node-C": {Id: "node-C", HeartBeatSeq: 1},
	// 	},
	// 	KV: map[string]KVEntry{
	// 		"leader":  {Value: "node-B", Version: 5},
	// 		"replica": {Value: "2", Version: 1},
	// 	},
	// }

	// a.merge(incoming)

	// fmt.Println("=== Members ===")
	// for id, node := range a.Members {
	// 	fmt.Printf("  %s → seq=%d\n", id, node.HeartBeatSeq)
	// }

	// fmt.Println("=== KV ===")
	// for key, entry := range a.KVStore {
	// 	fmt.Printf("  %s → value=%s version=%d\n", key, entry.Value, entry.Version)
	// }
}
