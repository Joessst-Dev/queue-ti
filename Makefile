.PHONY: proto deps test run build bench bench-mem bench-queue bench-loadtest install-hooks

proto:
	protoc --go_out=pb --go_opt=paths=source_relative \
		--go-grpc_out=pb --go-grpc_opt=paths=source_relative \
		--proto_path=proto \
		proto/queue.proto

deps:
	go mod tidy

install-hooks:
	cp scripts/commit-msg .git/hooks/commit-msg
	chmod +x .git/hooks/commit-msg
	@echo "commit-msg hook installed"

test:
	ginkgo ./...

build:
	go build -ldflags="-X main.version=$$(git describe --tags --always --dirty)" -o bin/queue-ti ./cmd/server

run:
	go run cmd/server/main.go

# Run all benchmarks (3 s per benchmark, no unit tests)
bench:
	go test -bench=. -benchtime=3s -run=^$$ ./internal/queue/...

# Run all benchmarks with memory allocation stats
bench-mem:
	go test -bench=. -benchtime=3s -benchmem -run=^$$ ./internal/queue/...

# Run a single named benchmark: make bench-queue BENCH=BenchmarkEnqueue
bench-queue:
	go test -bench=$(BENCH) -benchtime=3s -run=^$$ ./internal/queue/...

# Run the end-to-end gRPC load test against a running server
# Flags can be overridden: make bench-loadtest LOADTEST_FLAGS="--producers=16 --duration=60s"
bench-loadtest:
	go run ./cmd/loadtest $(LOADTEST_FLAGS)

