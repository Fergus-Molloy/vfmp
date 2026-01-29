set unstable

default: build lint

version := `git describe --tags --match='v[0-9].[0-9].[0-9]' HEAD 2>/dev/null || true`

version:
	echo {{ version }}

lint:
	golangci-lint run

build:
	go build -o vfmp -ldflags "-X fergus.molloy.xyz/vfmp/internal/version.Version={{version}}"

unit *flags:
	gotestsum --format=testname -- ./internal/... {{flags}}

watch:
	watchexec -r -e go,mod,sum -- just run

[script]
integration *flags:
	set -euo pipefail

	just build
	./vfmp -config "config.test.yml" > logs/integration.log 2>&1 &
	PID=$!
	trap "kill -s SIGTERM $PID 2>/dev/null || true" EXIT

	sleep 1 # wait for server to start

	gotestsum --format=testname ./tests/... -config "../config.test.yml" {{flags}}
	kill $PID 2>/dev/null || true

test: unit integration

[script]
run config="": build
	if [ -z "{{config}}" ]; then
		./vfmp -log-path logs/vfmp.log
	else
		./vfmp -log-path logs/vfmp.log -config "{{config}}"
	fi

[script]
client config="":
	if [ -z "{{config}}" ]; then
		go run client/main.go localhost:9090 test
	else
		go run client/main.go -config "{{config}}"
	fi

docker:
	docker build --build-arg VERSION={{version}} -t vfmp .

push: docker
	docker tag vfmp:latest git.molloy.xyz/fergus-molloy/vfmp:{{version}}
	docker push git.molloy.xyz/fergus-molloy/vfmp:{{version}}
