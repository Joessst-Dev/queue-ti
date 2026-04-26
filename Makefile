.PHONY: proto deps test run

proto:
	protoc --go_out=pb --go_opt=paths=source_relative \
		--go-grpc_out=pb --go-grpc_opt=paths=source_relative \
		--proto_path=proto \
		proto/queue.proto

deps:
	go mod tidy

test:
	ginkgo ./...

run:
	go run cmd/server/main.go

