.PHONY: all format lint vet test test-local check check-local

test = go test -v ./...

all:


format:
	test -z "$(goimports -l . | grep -v Godeps/_workspace/src/ | tee /dev/stderr)"

lint:
	test -z "$(golint ./... | tee /dev/stderr)"

vet:
	go vet ./...

test:
	docker run --rm --privileged -v $(shell pwd):/go/src/github.com/docker/libnetwork -w /go/src/github.com/docker/libnetwork golang:1.4 $(test)

test-local:
	$(test)

check:	format lint vet test

check-local: format lint vet test-local
