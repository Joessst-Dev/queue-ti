.PHONY: proto deps test run

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/queue.proto

deps:
	go mod tidy

test:
	ginkgo ./...

run:
	go run cmd/server/main.go

