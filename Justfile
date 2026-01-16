set unstable

default: build

version := `git describe --tags --match='v[0-9].[0-9].[0-9]' HEAD 2>/dev/null || true`

version:
	echo {{ version }}

build:
	go build -o vfmp

test:
	gotestsum --format=testname -- ./...

docker:
	docker build --build-arg VERSION={{version}} -t vfmp .

push: docker
	docker tag vfmp:latest git.molloy.xyz/fergus-molloy/vfmp:{{version}}
	docker push git.molloy.xyz/fergus-molloy/vfmp:{{version}}
