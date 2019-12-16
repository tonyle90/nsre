#!/bin/bash

# The nsre-alpine-build image is built localy using the Dockerfile.nsre-alpine-build

# docker build -t nsre-alpine-build:latest -f Dockerfile.nsre-alpine-build .

VER=$(git rev-parse --short HEAD)
sed -i "s/const Version = .*/const Version = \"${VER}\"/" cmd/version.go

#docker run --rm -v $(pwd):/work --workdir /work --entrypoint go nsre-alpine-build:latest build --tags "icu json1 fts5 secure_delete" --ldflags '-extldflags "-static" -w -s' -o nsre-linux-amd64-static

docker run --rm -v $(pwd):/work --workdir /work --entrypoint go --env-file ~/.gobuild-linux-cgo golang-alpine-build:latest build --tags "json1 secure_delete" --ldflags '-extldflags "-static" -w -s' -o nsre-linux-amd64-static main.go

mv nsre-linux-amd64-static ~/Public/nsre-linux-amd64-static
