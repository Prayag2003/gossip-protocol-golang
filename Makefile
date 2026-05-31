# ============================================================
#  Gossip Protocol — Makefile
#  Usage: make help
# ============================================================

BINARY   := gossip-node
BUILD_DIR := ./bin

# Default ports & IDs for the demo cluster
NODE_A_ID   := A
NODE_A_PORT := :8080

NODE_B_ID   := B
NODE_B_PORT := :8081

NODE_C_ID   := C
NODE_C_PORT := :8082

NODE_D_ID   := D
NODE_D_PORT := :8083

# Seed node is A (first node, no upstream seed)
SEED := $(NODE_A_PORT)

# ============================================================
.PHONY: help build clean \
        run-a run-b run-c run-d \
        cluster status-a status-b status-c status-d \
        kv-write kv-read

# ─── Help ───────────────────────────────────────────────────
help: ## Show this help message
	@echo ""
	@echo "  \033[1mGossip Protocol — available targets\033[0m"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""

# ─── Build ──────────────────────────────────────────────────
build: ## Compile the binary into ./bin/
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) .
	@echo "✅  Built → $(BUILD_DIR)/$(BINARY)"

clean: ## Remove compiled binaries
	rm -rf $(BUILD_DIR)
	@echo "🧹  Cleaned build artifacts"

# ─── Individual nodes ───────────────────────────────────────
run-a: build ## Start Node A (seed / bootstrap node)
	@echo "🚀  Starting Node A on $(NODE_A_PORT) (seed node — no upstream seed)"
	$(BUILD_DIR)/$(BINARY) $(NODE_A_ID) $(NODE_A_PORT) ""

run-b: build ## Start Node B (joins via seed Node A)
	@echo "🚀  Starting Node B on $(NODE_B_PORT) → seed $(SEED)"
	$(BUILD_DIR)/$(BINARY) $(NODE_B_ID) $(NODE_B_PORT) $(SEED)

run-c: build ## Start Node C (joins via seed Node A)
	@echo "🚀  Starting Node C on $(NODE_C_PORT) → seed $(SEED)"
	$(BUILD_DIR)/$(BINARY) $(NODE_C_ID) $(NODE_C_PORT) $(SEED)

run-d: build ## Start Node D (joins via seed Node A)
	@echo "🚀  Starting Node D on $(NODE_D_PORT) → seed $(SEED)"
	$(BUILD_DIR)/$(BINARY) $(NODE_D_ID) $(NODE_D_PORT) $(SEED)

# ─── Cluster (opens 4 terminal tabs, macOS) ─────────────────
cluster: build ## Launch a 4-node cluster in separate Terminal tabs (macOS)
	@echo "🌐  Launching 4-node cluster …"
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BUILD_DIR)/$(BINARY) $(NODE_A_ID) $(NODE_A_PORT) \"\""'
	@sleep 1
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BUILD_DIR)/$(BINARY) $(NODE_B_ID) $(NODE_B_PORT) $(SEED)"'
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BUILD_DIR)/$(BINARY) $(NODE_C_ID) $(NODE_C_PORT) $(SEED)"'
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BUILD_DIR)/$(BINARY) $(NODE_D_ID) $(NODE_D_PORT) $(SEED)"'
	@echo "✅  Cluster started — use 'make status-*' to inspect nodes"

# ─── Status endpoints ────────────────────────────────────────
status-a: ## Query /status on Node A
	@echo "📊  Node A status:"
	curl -s http://localhost$(NODE_A_PORT)/status | jq .

status-b: ## Query /status on Node B
	@echo "📊  Node B status:"
	curl -s http://localhost$(NODE_B_PORT)/status | jq .

status-c: ## Query /status on Node C
	@echo "📊  Node C status:"
	curl -s http://localhost$(NODE_C_PORT)/status | jq .

status-d: ## Query /status on Node D
	@echo "📊  Node D status:"
	curl -s http://localhost$(NODE_D_PORT)/status | jq .

# ─── KV store helpers ────────────────────────────────────────
# Usage:
#   make kv-write PORT=:8080 KEY=leader VALUE=nodeA
#   make kv-read  PORT=:8081 KEY=leader
kv-write: ## Write a KV pair to a node  (PORT= KEY= VALUE= required)
	@: $${PORT:=$(NODE_A_PORT)}
	curl -s -X POST http://localhost$(PORT)/kv \
	     -H "Content-Type: application/json" \
	     -d '{"Key":"$(KEY)","Value":"$(VALUE)"}' && echo ""

kv-read: ## Read a KV pair from a node  (PORT= KEY= required)
	@: $${PORT:=$(NODE_A_PORT)}
	curl -s http://localhost$(PORT)/kv/$(KEY) | jq .
