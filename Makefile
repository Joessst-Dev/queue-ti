.PHONY: proto proto-python deps test test-python setup-python run build bench bench-mem bench-queue bench-loadtest install-hooks up up-redis down build-nocache build-nocache-redis

proto:
	protoc --go_out=backend/pb --go_opt=paths=source_relative \
		--go-grpc_out=backend/pb --go-grpc_opt=paths=source_relative \
		--proto_path=proto \
		proto/queue.proto

proto-python:
	python3 -m grpc_tools.protoc \
		-I proto \
		--python_out=clients/python/queueti/pb \
		--grpc_python_out=clients/python/queueti/pb \
		proto/queue.proto
	sed -i '' 's/^import queue_pb2/from queueti.pb import queue_pb2/' \
		clients/python/queueti/pb/queue_pb2_grpc.py

deps:
	cd backend && go mod tidy

install-hooks:
	cp scripts/commit-msg .git/hooks/commit-msg
	chmod +x .git/hooks/commit-msg
	@echo "commit-msg hook installed"

test:
	cd backend && ginkgo ./...

setup-python:
	python3 -m venv clients/python/.venv
	clients/python/.venv/bin/pip install -e "clients/python[dev]"

test-python:
	clients/python/.venv/bin/pytest clients/python/tests/ --tb=short -q

build:
	cd backend && go build -ldflags="-X main.version=$$(git describe --tags --always --dirty)" -o ../bin/queue-ti ./cmd/server

run:
	cd backend && go run cmd/server/main.go

up:
	docker-compose up -d

up-redis:
	docker-compose -f docker-compose.yaml -f docker-compose.redis.yaml up -d

down:
	docker-compose -f docker-compose.yaml -f docker-compose.redis.yaml down

build-nocache:
	docker-compose build --no-cache

build-nocache-redis:
	docker-compose -f docker-compose.yaml -f docker-compose.redis.yaml build --no-cache

# Run all benchmarks (3 s per benchmark, no unit tests)
bench:
	cd backend && go test -bench=. -benchtime=3s -run=^$$ ./internal/queue/...

# Run all benchmarks with memory allocation stats
bench-mem:
	cd backend && go test -bench=. -benchtime=3s -benchmem -run=^$$ ./internal/queue/...

# Run a single named benchmark: make bench-queue BENCH=BenchmarkEnqueue
bench-queue:
	cd backend && go test -bench=$(BENCH) -benchtime=3s -run=^$$ ./internal/queue/...

# Run the end-to-end gRPC load test against a running server
# Flags can be overridden: make bench-loadtest LOADTEST_FLAGS="--producers=16 --duration=60s"
bench-loadtest:
	cd backend && go run ./cmd/loadtest $(LOADTEST_FLAGS)
