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

integration *flags:
	#!/usr/bin/env sh
	set -uo pipefail

	docker inspect vfmp:integration 2>/dev/null | \
		jq '.[].RepoTags[] == "vfmp:{{version}}"' | \
		grep true > /dev/null
	if [ "$?" -ne "0" ]; then
		just docker integration 2>/dev/null
	fi

	docker run -d --rm --name vfmp-integration -p 8081:8081 -v ./config.test.yml:/app/config.yml vfmp:integration -config /app/config.yml > /dev/null

	sleep 1 # wait for server to start

	gotestsum --format=testname ./tests/... -config "../config.test.yml" {{flags}}
	docker stop vfmp-integration 2>&1 > /dev/null

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

[script]
docker +tags="":
	docker build --build-arg VERSION={{version}} -t vfmp:latest .
	docker tag vfmp:latest vfmp:{{version}}
	for tag in {{tags}}; do
		docker tag vfmp:latest "vfmp:$tag"
	done


push: docker
	just docker git.molloy.xyz/fergus-molloy/vfmp:{{version}}
	docker push git.molloy.xyz/fergus-molloy/vfmp:{{version}}
