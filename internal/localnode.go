package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"
)

var SECOND = time.Second

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

func (ln *LocalNode) merge(msg GossipMessage) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	// For Heartbeat
	for key, incomingNode := range msg.Members {
		if key == ln.Self.Id {
			continue
		}

		existing, ok := ln.Members[key]
		incomingNode.LastSeen = time.Now()
		if !ok {
			ln.Members[key] = incomingNode
			slog.Info("Discovered new member", "id", key, "addr", incomingNode.Addr)
		} else if incomingNode.HeartBeatSeq > existing.HeartBeatSeq {
			ln.Members[key] = incomingNode
			if existing.Status != StatusAlive && incomingNode.Status == StatusAlive {
				// resurrection
				slog.Info("Node Resurrected", "id", key)
			}
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

func (ln *LocalNode) GossipHandler(w http.ResponseWriter, r *http.Request) {
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

// I want to gossip to others
func (ln *LocalNode) PushPull(peer Node) error {
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

func (ln *LocalNode) GetStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ln.buildGossipMessage())
}

func (ln *LocalNode) Heartbeat() {
	ln.mu.Lock()
	slog.Info("Incrementing Heartbeat")
	ln.Self.HeartBeatSeq++
	ln.Self.LastSeen = time.Now()
	ln.Members[ln.Self.Id] = ln.Self
	ln.mu.Unlock()
}

func (ln *LocalNode) AlivePeers() []Node {
	ln.mu.RLock()

	// collect non-self members into a slice
	var peers []Node
	for id, peer := range ln.Members {
		if id != ln.Self.Id && peer.Status == StatusAlive {
			peers = append(peers, peer)
		}
	}
	ln.mu.RUnlock()
	return peers
}

func (ln *LocalNode) Reap() {
	ln.mu.Lock()
	for id, peer := range ln.Members {
		if id == ln.Self.Id {
			continue
		}

		switch peer.Status {
		case StatusAlive:
			if time.Since(peer.LastSeen) > 30*SECOND {
				peer.Status = StatusSuspcted
				peer.SuspectedAt = time.Now()
				ln.Members[id] = peer
				slog.Warn("Node", "id", id, "lastSeen", peer.LastSeen)
			}

		case StatusSuspcted:
			if time.Since(peer.SuspectedAt) > 15*SECOND {
				peer.Status = StatusDead
				ln.Members[id] = peer
				slog.Warn("Node", "id", id)
			}

		case StatusDead:
			if time.Since(peer.SuspectedAt) > 60*SECOND {
				delete(ln.Members, id)
				slog.Warn("Node removed from members", "id", id)
			}
		}
	}
	ln.mu.Unlock()
}

func (ln *LocalNode) KVWriteHandler(w http.ResponseWriter, r *http.Request) {
	var req KVWriteRequest

	const MB = 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, 1*MB)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	ln.mu.Lock()
	existing, ok := ln.KVStore[req.Key]
	newVersion := uint64(1)
	if ok {
		newVersion = existing.Version + 1
	}
	ln.KVStore[req.Key] = KVEntry{Value: req.Value, Version: newVersion}
	ln.mu.Unlock()

	slog.Info("KV write", "key", req.Key, "value", req.Value, "version", newVersion)
	w.WriteHeader(http.StatusOK)
}

func (ln *LocalNode) KVReadHandler(w http.ResponseWriter, r *http.Request) {
	// strips "/kv/" prefix to get the key
	// e.g. /kv/leader → leader
	key := strings.TrimPrefix(r.URL.Path, "/kv/")
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	ln.mu.RLock()
	entry, ok := ln.KVStore[key]
	ln.mu.RUnlock()

	if !ok {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(KVReadResponse{
		Key:     key,
		Value:   entry.Value,
		Version: entry.Version,
	})
}
