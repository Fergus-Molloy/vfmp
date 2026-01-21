# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VFMP (Very Fast Message Processor) is a minimal Go HTTP service that provides:
- Health check endpoint (`/control/healthcheck`)
- Version endpoint (`/control/version`) - version is set at compile time via ldflags
- Echo endpoint (`/echo`) - reads up to 50 bytes from request body and echoes it back

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
just build  # Default - builds binary to ./vfmp
just        # Alias for 'just build'
```

### Testing
```bash
just test   # Runs gotestsum with testname format
```

### Linting
```bash
just lint   # Runs golangci-lint
```

### Running Locally
```bash
just run    # Builds and runs the server
```

### Docker
```bash
just docker # Builds Docker image with version from git tags
just push   # Builds, tags, and pushes to registry
```

### Version
```bash
just version  # Shows version from git tags (format: v[0-9].[0-9].[0-9])
```

### Direct Go Commands (if needed)
```bash
go build -o vfmp
go test ./...
golangci-lint run
```

## Architecture

### Package Structure
- `main.go` - Entry point with HTTP server setup and route handlers
- `internal/http/http.go` - HTTP server utilities with graceful shutdown support
  - `StartHttpServer()` - Creates and starts an HTTP server with proper timeouts
  - `StartPprofServer()` - Starts a pprof debugging server (imports `net/http/pprof`)

### Key Patterns
- Version is injected at compile time via `-ldflags "-X main.version=$VERSION"`
- HTTP endpoints use standard library `http.HandleFunc`
- Server runs on port 8080 by default
- The `internal/http` package provides server lifecycle management with `sync.WaitGroup` for coordinated shutdown

### CI/CD
The project uses Forgejo Actions (GitHub Actions-compatible) with three jobs:
1. **lint** - Runs golangci-lint v2.8
2. **build** - Builds the binary with `go build main.go`
3. **test** - Runs tests using gotestsum with testname format

Workflows are triggered on pushes to main and all pull requests.
