package main

import (
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/Prayag2003/gossip-protocol-golang/internal"
)

var SECOND = time.Second

func main() {
	id := os.Args[1]
	addr := os.Args[2]
	seed := os.Args[3]

	node := internal.NewLocalNode(id, addr)

	if seed != "" {
		seedNode := internal.NewNode("seed", seed)
		if err := node.PushPull(seedNode); err != nil {
			log.Printf("initial seed contact failed: %v", err)
		}
	}

	// Heartbeat Tick
	go func() {
		for range time.Tick(3 * SECOND) {
			node.Heartbeat()
		}
	}()

	const K = 3

	// Gossip Tick
	go func() {
		for range time.Tick(10 * SECOND) {
			slog.Info("Starting gossip using ticks")
			peers := node.AlivePeers()
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
				go func(p internal.Node) {
					if err := node.PushPull(p); err != nil {
						fmt.Println(fmt.Errorf("pushPull failed to %s: %v", p.Id, err.Error()))
					}
				}(peer)
			}

		}
	}()

	// Reaper Tick
	go func() {
		for range time.Tick(5 * SECOND) {
			node.Reap()
		}
	}()

	http.HandleFunc("/gossip", node.GossipHandler)
	http.HandleFunc("/status", node.GetStatus)
	log.Fatal(http.ListenAndServe(addr, nil))
}
