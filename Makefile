# sushi-webui build. Node is only needed here, on the dev machine.
SUSHI_PROTO_DIR ?= ../sushi/rpc_interface/protos
GOBIN := $(shell go env GOPATH)/bin
MODULE := github.com/elk-audio/sushi-webui/internal/sushirpc
PROTO_MAP := Msushi_rpc.proto=$(MODULE),Mapi_version.proto=$(MODULE)

.PHONY: all web backend run dev proto test check clean arm64

all: web backend

web:
	cd web && npm install --no-audit --no-fund && npm run build

backend:
	go build -trimpath -o bin/sushi-webui ./cmd/sushi-webui

# Cross-compile for Elk Audio OS on Raspberry Pi 4 (static, no runtime dependencies).
arm64: web
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/sushi-webui-linux-arm64 ./cmd/sushi-webui

run: all
	./bin/sushi-webui

# Backend with relaxed Origin checks + Vite dev server (hot reload) on :5173.
dev:
	go run ./cmd/sushi-webui --dev & cd web && npm run dev

proto:
	cp $(SUSHI_PROTO_DIR)/sushi_rpc.proto $(SUSHI_PROTO_DIR)/api_version.proto proto/
	PATH=$(GOBIN):$$PATH protoc -I proto \
		--go_out=internal/sushirpc --go_opt=paths=source_relative,$(PROTO_MAP) \
		--go-grpc_out=internal/sushirpc --go-grpc_opt=paths=source_relative,$(PROTO_MAP) \
		sushi_rpc.proto api_version.proto

test:
	go test -race ./...

check:
	go vet ./... && go test ./... && cd web && npm run check

clean:
	rm -rf bin web/dist/assets
