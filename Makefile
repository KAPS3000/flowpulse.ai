SHELL := /bin/bash
.DEFAULT_GOAL := build

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/flowpulse/flowpulse/internal/version.Version=$(VERSION) \
           -X github.com/flowpulse/flowpulse/internal/version.Commit=$(COMMIT) \
           -X github.com/flowpulse/flowpulse/internal/version.Date=$(DATE)

BPF_CLANG  ?= clang
BPF_CFLAGS := -O2 -g -Wall -Werror -target bpf -D__TARGET_ARCH_x86

.PHONY: generate bpf proto build test lint clean docker-agent docker-aggregator docker-server docker-web docker

## ── BPF ──────────────────────────────────────────────────────

bpf: bpf/headers/vmlinux.h
	$(BPF_CLANG) $(BPF_CFLAGS) -I bpf/headers -c bpf/flow_tracker.c -o bpf/flow_tracker.o
	$(BPF_CLANG) $(BPF_CFLAGS) -I bpf/headers -c bpf/cpu_sched.c    -o bpf/cpu_sched.o
	$(BPF_CLANG) $(BPF_CFLAGS) -I bpf/headers -c bpf/ib_verbs.c     -o bpf/ib_verbs.o

bpf/headers/vmlinux.h:
	@echo "Generating vmlinux.h from BTF..."
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > $@

generate: bpf
	go generate ./...

## ── Protobuf ─────────────────────────────────────────────────

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       proto/*.proto

## ── Build ────────────────────────────────────────────────────

build: generate
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/flowpulse-agent      ./cmd/agent
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/flowpulse-aggregator ./cmd/aggregator
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/flowpulse-server     ./cmd/server
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/fpctl                ./cmd/fpctl

build-agent:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/flowpulse-agent ./cmd/agent

build-aggregator:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/flowpulse-aggregator ./cmd/aggregator

build-server:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/flowpulse-server ./cmd/server

## ── Test / Lint ──────────────────────────────────────────────

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

## ── Docker ───────────────────────────────────────────────────

docker-agent:
	docker build -f docker/Dockerfile.agent -t flowpulse-agent:$(VERSION) .

docker-aggregator:
	docker build -f docker/Dockerfile.aggregator -t flowpulse-aggregator:$(VERSION) .

docker-server:
	docker build -f docker/Dockerfile.server -t flowpulse-server:$(VERSION) .

docker-web:
	docker build -f docker/Dockerfile.web -t flowpulse-web:$(VERSION) .

docker: docker-agent docker-aggregator docker-server docker-web

## ── Clean ────────────────────────────────────────────────────

clean:
	rm -rf bin/ bpf/*.o
