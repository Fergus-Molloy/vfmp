set unstable

default: build lint

version := `git describe --tags --match='v[0-9].[0-9].[0-9]' HEAD 2>/dev/null || true`

version:
	@echo {{ version }}

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

[script]
run config="" *flags="-log-path logs/vfmp.log": build
	if [ -z "{{config}}" ]; then
		./bin/vfmp {{flags}}
	else
		./bin/vfmp -config "{{config}}" {{flags}}
	fi

start config="": build
	just run {{config}} > logs/vfmp.log 2>&1 &

stop:
	pkill vfmp

[script]
client config="": (build "client")
	if [ -z "{{config}}" ]; then
		./bin/client localhost:9090 test
	else
		./bin/client -config "{{config}}"
	fi

cli *args="test": (build "cli")
	./bin/cli localhost:9090 {{args}}

[script]
docker +tags="":
	docker build -f deploy/Dockerfile --build-arg VERSION={{version}} -t vfmp:latest .
	docker tag vfmp:latest vfmp:{{version}}
	for tag in {{tags}}; do
		docker tag vfmp:latest "vfmp:$tag"
	done


push: docker
	just docker git.molloy.xyz/fergus-molloy/vfmp:{{version}}
	docker push git.molloy.xyz/fergus-molloy/vfmp:{{version}}
