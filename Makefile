BINARY    := gossip-node
BUILD_DIR := ./bin
SEED      := :8080

.PHONY: help build clean run-a run-b run-c run-d cluster node-a node-b node-c node-d kv-write kv-read

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Compile → ./bin/gossip-node
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) .

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

run-a: build ## Start Node A (seed)
	$(BUILD_DIR)/$(BINARY) Node-A :8080 ""

run-b: build ## Start Node B
	$(BUILD_DIR)/$(BINARY) Node-B :8081 $(SEED)

run-c: build ## Start Node C
	$(BUILD_DIR)/$(BINARY) Node-C :8082 $(SEED)

run-d: build ## Start Node D
	$(BUILD_DIR)/$(BINARY) Node-D :8083 $(SEED)

cluster: build ## Launch 4-node cluster in Terminal tabs (macOS)
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BUILD_DIR)/$(BINARY) Node-A :8080 \"\""'
	@sleep 1
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BUILD_DIR)/$(BINARY) Node-B :8081 $(SEED)"'
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BUILD_DIR)/$(BINARY) Node-C :8082 $(SEED)"'
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BUILD_DIR)/$(BINARY) Node-D :8083 $(SEED)"'

node-a: ## GET /status from Node A
	curl -s http://localhost:8080/status | jq .

node-b: ## GET /status from Node B
	curl -s http://localhost:8081/status | jq .

node-c: ## GET /status from Node C
	curl -s http://localhost:8082/status | jq .

node-d: ## GET /status from Node D
	curl -s http://localhost:8083/status | jq .

# make kv-write PORT=:8080 KEY=foo VALUE=bar
kv-write: ## Write KV pair  (PORT= KEY= VALUE=)
	curl -s -X POST http://localhost$(PORT)/kv \
	     -H "Content-Type: application/json" \
	     -d '{"Key":"$(KEY)","Value":"$(VALUE)"}' && echo ""

# make kv-read PORT=:8081 KEY=foo
kv-read: ## Read KV pair   (PORT= KEY=)
	curl -s http://localhost$(PORT)/kv/$(KEY) | jq .
