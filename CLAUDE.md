# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VFMP (Very Fast Message Processor) is a Go-based message broker/queue system that provides:
- **Topic-based message queuing**: Publish messages to topics via HTTP, consume via TCP
- **HTTP API**: Publish messages, check queue counts, peek messages
- **TCP Server**: Clients connect, send RDY messages, receive messages, respond with ACK/NCK/DLQ
- **Prometheus Metrics**: Queue lengths, message counts, HTTP latency tracking
- **Control Endpoints**: Health check (`/control/healthcheck`) and version (`/control/version`)
- **Graceful Shutdown**: Coordinated shutdown of all servers with timeout support

## Development Setup

This project uses Nix flakes for dependency management. To set up the development environment:

```bash
nix develop
# Or if using direnv:
direnv allow
```

The Nix flake provides: `just`, `go`, `gopls`, `golangci-lint`, `golangci-lint-langserver`, and `gotestsum`.

## Common Commands

The project uses a Justfile for common tasks. Prefer these commands:

### Building
```bash
just build               # Builds main server (vfmp) to ./bin/vfmp
just build cli           # Builds CLI tool to ./bin/cli
just build client        # Builds TCP client to ./bin/client
just                     # Alias for 'just build lint'
```

### Testing
```bash
just test                # Runs both unit and integration tests
just unit                # Runs unit tests (./internal/...) with gotestsum
just integration         # Builds server, starts it, runs integration tests (./tests/...)
just watch [recipes...]  # Watch for file changes and re-run recipes
```

### Linting
```bash
just lint                # Runs golangci-lint
```

### Running Locally
```bash
just run [config]        # Builds and runs the server (optionally with config file)
just start [config]      # Starts server in background
just stop                # Stops background server (pkill vfmp)
```

### CLI Tools
```bash
just cli [args]          # Runs CLI tool (default: localhost:9090 test)
just client [config]     # Runs TCP client (default: localhost:9090 test)
```

### Docker & Deployment
```bash
just docker [tags...]    # Builds Docker image with version from git tags
just push                # Builds and pushes image to registry
```

### Version
```bash
just version             # Shows version from git tags (format: v[0-9].[0-9].[0-9])
```

## Architecture

### Package Structure

**Commands** (`cmd/`):
- `cmd/vfmp/main.go` - Main server entry point
- `cmd/cli/main.go` - CLI tool for interacting with server
- `cmd/client/main.go` - TCP client for consuming messages

**Internal Packages** (`internal/`):
- `broker/` - Message broker managing topics and routing messages to queues
- `queue/` - Linked-list based queue implementation per topic
- `http/` - HTTP server with handlers, middlewares (logging, correlation ID), graceful shutdown
- `tcp/` - TCP server handling client connections and message ACK/NCK/DLQ protocol
- `config/` - YAML/env var/flag-based configuration loading
- `metrics/` - Prometheus metrics registration and collection
- `logger/` - Tee logger for writing to both stdout and file
- `version/` - Version package (version injected at compile time)
- `model/` - Message and list data structures

**Core Packages** (`core/`):
- `tcp/` - Low-level TCP client utilities
- `messages/` - Message parsing and formatting (RDY, ACK, NCK, DLQ, MSG)

**Tests** (`tests/`):
- Integration tests for HTTP and message handling

### Key Patterns

**Configuration**:
- YAML config files, environment variables, and command-line flags
- Priority: flags > env vars > config file > defaults
- Default ports: HTTP :8080, TCP :9090, Metrics :5050

**Version Management**:
- Version injected at compile time: `-ldflags "-X fergus.molloy.xyz/vfmp/internal/version.Version=$VERSION"`
- Version format from git tags: `v[0-9].[0-9].[0-9]`

**Message Flow**:
1. HTTP POST to `/messages/{topic}` with `X-Correlation-ID` header
2. Message added to broker's channel
3. Broker routes message to topic-specific queue
4. TCP clients send RDY message for topic
5. Server sends MSG to client
6. Client responds with ACK (success), NCK (retry), or DLQ (dead letter)
7. Server tracks in-flight messages per client with 10-second timeout

**Concurrency**:
- `sync.WaitGroup` for coordinated shutdown across all servers
- Context-based cancellation for graceful cleanup
- Mutex-protected broker topics map and per-client message tracking

**Metrics**:
- Separate metrics server on :5050 with `/metrics` endpoint
- Tracks: topic count, queue lengths, message in/ack/nck/dlq counts, HTTP latency

## API Reference

### HTTP Endpoints

**Control Endpoints**:
- `GET /control/healthcheck` - Returns 200 OK
- `GET /control/version` - Returns version string

**Message Endpoints**:
- `POST /messages/{topic}` - Publish a message to a topic
  - Requires `X-Correlation-ID` header (UUID format)
  - Body: raw message bytes
- `GET /messages/{topic}?data=count` - Get message count for a topic
  - Returns: `{"count": <int>}`
- `GET /messages/{topic}?data=peek` - Peek at next message without consuming
  - Returns: JSON message object with `body`, `topic`, `correlationID`, `createdAt`

**Metrics Endpoint**:
- `GET /metrics` (on metrics server :5050) - Prometheus metrics

### TCP Protocol

**Message Format**: `<TYPE> <TOPIC> [CORRELATION_ID]\n`

**Client-to-Server Messages**:
- `RDY <topic>` - Client is ready to receive a message from topic
- `ACK <topic> <correlation_id>` - Successfully processed message
- `NCK <topic> <correlation_id>` - Failed to process, requeue message
- `DLQ <topic> <correlation_id>` - Dead letter the message (discard)

**Server-to-Client Messages**:
- `MSG <topic> <correlation_id>\n<length>\n<body>` - Deliver message to client

**Behavior**:
- Messages are tracked per client until ACK/NCK/DLQ received
- 10-second timeout for ACK/NCK/DLQ (auto-NCK on timeout)
- On client disconnect, all in-flight messages are NCK'd (requeued)

### CI/CD

The project uses Forgejo Actions (GitHub Actions-compatible) defined in `.forgejo/workflows/build.yml`:

**Jobs**:
1. **lint** - Runs `gofmt` check and `golangci-lint` v2.8
2. **build** - Builds the main binary: `go build -o vfmp ./cmd/vfmp/main.go`
3. **unit** - Runs unit tests: `gotestsum --format=testname ./internal/...`
4. **integration** - Starts server, runs integration tests: `gotestsum --format=testname ./tests/...`
5. **build-and-push-image** - Builds and pushes Docker image to `git.molloy.xyz` registry
6. **deploy** - Deploys to Kubernetes (only on manual workflow_dispatch with environment selection)

**Triggers**:
- Pushes to `main` branch
- All pull requests
- Git tags matching `v*`
- Manual workflow dispatch (for deployments)

**Docker & Deployment**:
- Docker images tagged with `latest` (on main) and semver (on tags)
- Kubernetes deployment via kustomize overlays (`deploy/k8s/overlays/{dev,prod}`)
- Rollout restart after deployment to pull latest image
