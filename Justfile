default: build lint

version := `git describe --tags --match='v[0-9].[0-9].[0-9]' HEAD 2>/dev/null || true`

version:
	@echo {{ version }}

proto:
	protoc \
		--proto_path=proto \
		--proto_path=/usr/include \
		--go_out=gen \
		--go_opt=paths=source_relative \
		--go-grpc_out=gen \
		--go-grpc_opt=paths=source_relative \
		$(find proto -name '*.proto')

lint:
	golangci-lint run

build target="vfmp":
	go build -o bin/{{target}} -ldflags "-X fergus.molloy.xyz/vfmp/internal/version.Version={{version}}" "./cmd/{{target}}/main.go"

unit *flags:
	gotestsum --format=testname -- ./internal/... {{flags}}

watch *recipes="":
	watchexec -r -e go,mod,sum -- just {{recipes}}

integration *flags: build
	#!/usr/bin/env sh
	set -uo pipefail

	./bin/vfmp -config "config.test.yml" > logs/integration.log 2>&1 &
	PID=$!
	trap "kill -s SIGTERM $PID 2>/dev/null || true" EXIT

	sleep 1 # wait for server to start

	gotestsum --format=testname ./tests/... -config "../config.test.yml" {{flags}}
	kill $PID 2>/dev/null || true


test: unit integration

run config="" *flags="-log-path logs/vfmp.log": build
	#!/usr/bin/env bash

	if [ -z "{{config}}" ]; then
		./bin/vfmp {{flags}}
	else
		./bin/vfmp -config "{{config}}" {{flags}}
	fi

start config="": build
	just run {{config}} > logs/vfmp.log 2>&1 &

stop:
	pkill vfmp

client *args: (build "client")
		./bin/client {{args}}

cli *args="": (build "cli")
	./bin/cli {{args}}

producer *args="": (build "producer")
	./bin/producer {{args}}

docker +tags="":
	#!/usr/bin/env bash

	docker build -f deploy/Dockerfile --build-arg VERSION={{version}} -t vfmp:latest .
	docker tag vfmp:latest vfmp:{{version}}
	for tag in {{tags}}; do
		docker tag vfmp:latest "$tag"
	done


push: docker
	just docker git.molloy.xyz/fergus-molloy/vfmp:{{version}}
	docker push git.molloy.xyz/fergus-molloy/vfmp:{{version}}
