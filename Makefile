.PHONY: build run test bench lint proto migrate-up migrate-down

build:
	go build -o bin/worldnode.exe cmd/worldnode/main.go
	go build -o bin/gateway.exe cmd/gateway/main.go

run:
	go run cmd/worldnode/main.go

test:
	go test -v ./...

bench:
	go test -bench=. -benchmem ./...

lint:
	golangci-lint run

proto:
	powershell ./scripts/proto-gen.ps1

migrate-up:
	go run cmd/tools/migrate/main.go up

migrate-down:
	go run cmd/tools/migrate/main.go down
