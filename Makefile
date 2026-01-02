# CoreDNS version to use
COREDNS_VERSION ?= v1.13.2

# Directories
COREDNS_DIR = ./coredns
BUILD_DIR = ./build

.PHONY: test
test:
	go test -v ./...

.PHONY: test-race
test-race:
	go test -race -v ./...

.PHONY: test-integration
test-integration:
	go test -v -run TestIntegration ./...

.PHONY: test-all
test-all: test test-integration

.PHONY: coredns-clone
coredns-clone:
	@if [ ! -d "$(COREDNS_DIR)" ]; then \
		echo "Cloning CoreDNS $(COREDNS_VERSION)..."; \
		git clone --depth 1 --branch $(COREDNS_VERSION) https://github.com/coredns/coredns.git $(COREDNS_DIR); \
	else \
		echo "CoreDNS already cloned at $(COREDNS_DIR)"; \
	fi

.PHONY: coredns-register
coredns-register: coredns-clone
	@echo "Registering elchi plugin in CoreDNS..."
	@if ! grep -q "elchi:github.com/cloudnativeworks/elchi-gslb" $(COREDNS_DIR)/plugin.cfg; then \
		awk '/^kubernetes:/ {print; print "elchi:github.com/cloudnativeworks/elchi-gslb"; next} 1' \
			$(COREDNS_DIR)/plugin.cfg > $(COREDNS_DIR)/plugin.cfg.tmp && \
		mv $(COREDNS_DIR)/plugin.cfg.tmp $(COREDNS_DIR)/plugin.cfg; \
		echo "Plugin registered in plugin.cfg"; \
	else \
		echo "Plugin already registered"; \
	fi
	@cd $(COREDNS_DIR) && go mod edit -replace github.com/cloudnativeworks/elchi-gslb=../ && go get github.com/cloudnativeworks/elchi-gslb && go mod tidy
	@echo "Generating plugin loader code..."
	@cd $(COREDNS_DIR) && go generate

.PHONY: coredns-build
coredns-build: coredns-register
	@echo "Building CoreDNS with elchi plugin..."
	@cd $(COREDNS_DIR) && go generate && go build -o ../coredns-elchi
	@echo "Built: ./coredns-elchi"

.PHONY: build
build: coredns-build

.PHONY: run
run: coredns-register
	@if [ ! -f Corefile ]; then \
		echo "Error: Corefile not found. Run 'make setup' first."; \
		exit 1; \
	fi
	@echo "Starting mock Elchi controller on :1052..."
	@go run ./mock-controller &
	@echo $$! > .mock-controller.pid
	@sleep 2
	@echo "Starting CoreDNS with elchi plugin on port 1053..."
	@cd $(COREDNS_DIR) && go run . -conf ../Corefile || (kill `cat ../.mock-controller.pid` 2>/dev/null; rm -f ../.mock-controller.pid; exit 1)

.PHONY: stop
stop:
	@echo "Stopping all servers..."
	@if [ -f .mock-controller.pid ]; then \
		kill `cat .mock-controller.pid` 2>/dev/null || true; \
		rm -f .mock-controller.pid; \
	fi
	@pkill -f "go run.*mock-controller" || true
	@pkill -f "go run.*coredns" || true
	@lsof -ti:1052 | xargs kill -9 2>/dev/null || true
	@lsof -ti:1053 | xargs kill -9 2>/dev/null || true
	@echo "Stopped all servers"

.PHONY: run-build
run-build: build
	@if [ ! -f Corefile ]; then \
		echo "Error: Corefile not found. Run 'make setup' first."; \
		exit 1; \
	fi
	@echo "Starting CoreDNS from built binary (requires sudo for port 53)..."
	sudo ./coredns-elchi -conf Corefile

.PHONY: setup
setup:
	@echo "Setting up elchi-gslb development environment..."
	@if [ ! -f Corefile ]; then \
		cp Corefile.example Corefile; \
		echo "✓ Created Corefile from example"; \
	else \
		echo "✓ Corefile already exists"; \
	fi
	@go mod download && go mod tidy
	@echo "✓ Go dependencies downloaded"
	@$(MAKE) coredns-clone
	@echo ""
	@echo "✅ Setup complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Edit Corefile with your zone and secret"
	@echo "  2. Run: make run          (go run - no build needed)"
	@echo "  3. Test: make query"
	@echo ""
	@echo "Or build binary first:"
	@echo "  1. make build"
	@echo "  2. make run-build"

.PHONY: query
query:
	@echo "Testing DNS query..."
	@dig @localhost -p 1053 listener1.gslb.elchi A +short

.PHONY: query-health
query-health:
	@echo "Checking plugin health..."
	@curl -s http://localhost:8053/health | jq '.' || echo "Webhook server not enabled or not running"

.PHONY: query-records
query-records:
	@echo "Fetching cached records..."
	@curl -s -H "X-Elchi-Secret: test-secret-key" http://localhost:8053/records | jq '.' || echo "Webhook server not enabled or auth failed"

.PHONY: query-controller
query-controller:
	@echo "Checking mock controller health..."
	@curl -s http://localhost:1052/health | jq '.' || echo "Mock controller not running"

.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -f coredns-elchi
	rm -rf $(BUILD_DIR)
	rm -f .mock-controller.pid
	go clean

.PHONY: clean-all
clean-all: clean
	@echo "Removing CoreDNS clone..."
	rm -rf $(COREDNS_DIR)
	@echo "Removing Corefile..."
	rm -f Corefile

.PHONY: dev
dev: setup
	@echo ""
	@echo "✅ Development environment ready!"
	@echo ""
	@echo "Quick start:"
	@echo "  make run        - Run with go run (fast, no build)"
	@echo "  make run-build  - Run from binary (requires make build first)"

.PHONY: help
help:
	@echo "Elchi GSLB CoreDNS Plugin - Development Commands"
	@echo ""
	@echo "Setup & Build:"
	@echo "  make setup           - First-time setup (clone CoreDNS, create Corefile)"
	@echo "  make dev             - Full dev setup (setup only, no build)"
	@echo "  make build           - Build CoreDNS binary with elchi plugin"
	@echo ""
	@echo "Development (with Mock Controller):"
	@echo "  make run             - Run DNS server + mock controller [RECOMMENDED]"
	@echo "  make stop            - Stop all running servers"
	@echo "  make test            - Run unit tests"
	@echo "  make test-race       - Run tests with race detector"
	@echo "  make test-integration - Run integration tests"
	@echo "  make test-all        - Run all tests (unit + integration)"
	@echo ""
	@echo "Testing & Monitoring:"
	@echo "  make query           - Test DNS query (listener1.gslb.elchi)"
	@echo "  make query-health    - Check plugin health (webhook)"
	@echo "  make query-records   - List cached records (webhook)"
	@echo "  make query-controller - Check mock controller health"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean           - Clean build artifacts"
	@echo "  make clean-all       - Clean everything (including CoreDNS clone)"
	@echo ""
	@echo "Configuration:"
	@echo "  COREDNS_VERSION      - CoreDNS version (default: $(COREDNS_VERSION))"
	@echo "  Port 1053            - DNS server (no sudo required)"
	@echo "  Port 1052            - Mock controller API"
	@echo "  Port 8053            - Webhook management API"
	@echo "  Port 9253            - Prometheus metrics"
	@echo ""
	@echo "Quick start:"
	@echo "  1. make setup        # Clone CoreDNS, create Corefile"
	@echo "  2. make run          # Start mock controller + DNS server"
	@echo "  3. make query        # Test in another terminal"
	@echo "  4. make stop         # Stop all servers when done"
