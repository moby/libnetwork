ARG GO_VERSION=1.18.9

FROM golang:${GO_VERSION}-buster as dev
RUN apt-get update && apt-get -y install iptables \
		protobuf-compiler

RUN git clone https://github.com/gogo/protobuf.git  /go/src/github.com/gogo/protobuf \
  && cd /go/src/github.com/gogo/protobuf/protoc-gen-gogo \
  && git reset --hard 30cf7ac33676b5786e78c746683f0d4cd64fa75b \
  && GO111MODULE=off go install

RUN go install golang.org/x/lint/golint@latest \
 && go install golang.org/x/tools/cmd/cover@latest \
 && go install github.com/mattn/goveralls@latest \
 && go install github.com/gordonklaus/ineffassign@latest \
 && go install github.com/client9/misspell/cmd/misspell@latest

WORKDIR /go/src/github.com/docker/libnetwork
ENV GO111MODULE=off


FROM dev

COPY . .
