module github.com/Joessst-Dev/queue-ti/tools/seeder

go 1.25.5

require (
	github.com/Joessst-Dev/queue-ti/clients/go-client v0.0.0
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/Joessst-Dev/queue-ti v0.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/grpc v1.80.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	github.com/Joessst-Dev/queue-ti => ../../backend
	github.com/Joessst-Dev/queue-ti/clients/go-client => ../../clients/go-client
)
