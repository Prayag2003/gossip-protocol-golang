BIN  := ./bin/gossip-node
SEED := :8080

.PHONY: build clean run node cluster kv-write kv-read

build:
	@mkdir -p ./bin && go build -o $(BIN) .

clean:
	rm -rf ./bin

# make run ID=Node-A PORT=:8080 [SEED=:8080]
run: build
	$(BIN) $(ID) $(PORT) $(or $(SEED_ARG),"")

# make node PORT=:8080
node:
	curl -s http://localhost$(PORT)/status | jq .

cluster: build
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BIN) Node-A :8080 \"\""'
	@sleep 1
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BIN) Node-B :8081 $(SEED)"'
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BIN) Node-C :8082 $(SEED)"'
	osascript -e 'tell application "Terminal" to do script "cd $(CURDIR) && $(BIN) Node-D :8083 $(SEED)"'

# make kv-write PORT=:8080 KEY=foo VALUE=bar
kv-write:
	curl -s -X POST http://localhost$(PORT)/kv -H "Content-Type: application/json" -d '{"Key":"$(KEY)","Value":"$(VALUE)"}' && echo ""

# make kv-read PORT=:8081 KEY=foo
kv-read:
	curl -s http://localhost$(PORT)/kv/$(KEY) | jq .
