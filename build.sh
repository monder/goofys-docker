#!/bin/sh
docker run --rm -v "$PWD":/go/src/github.com/monder/goofys-docker -w /go/src/github.com/monder/goofys-docker -it -e GO15VENDOREXPERIMENT=1 golang:1.5 bash -c "go get github.com/Masterminds/glide && glide i && go build -a"
