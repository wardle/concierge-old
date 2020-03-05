
ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
BINARY=concierge
VERSION=0.1
BUILD=`git rev-list HEAD --max-count=1 --abbrev-commit`
PLATFORMS=darwin linux windows
ARCHITECTURES=amd64
GOOGLEAPIS=${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.build=${BUILD}"

default: generate test build

all: generate test build build_all

generate:
	mkdir -p apiv1
	protoc -Iprotos/concierge-api/v1 -I${GOOGLEAPIS} --go_out=plugins=grpc:${GOPATH}/src model.proto
	protoc -Iprotos/concierge-api/v1 -I${GOOGLEAPIS} --go_out=plugins=grpc:${GOPATH}/src empi.proto
	protoc -Iprotos/concierge-api/v1 -I${GOOGLEAPIS} --grpc-gateway_out=logtostderr=true:${GOPATH}/src empi.proto
#	protoc -Ivendor/concierge-api/v1 -I${GOOGLEAPIS} --swagger_out=logtostderr=true:. concierge.proto

generate-jar:
	protoc -Iprotos/v1 -I${GOOGLEAPIS} --plugin=protoc-gen-grpc-java=/usr/local/bin/protoc-gen-grpc-java-1.27.2-osx-x86_64.exe --grpc-java_out=wibble --java_out=concierge-protos-v${VERSION}.jar concierge.proto

bench:
	go test -bench=.  ./concierge

test:
	@go test -cover ./...

test-nc:
	@go test ./... -count=1

build:
	@go build $(LDFLAGS) -o ${BINARY}

build_all:
	$(foreach GOOS, $(PLATFORMS),\
	$(foreach GOARCH, $(ARCHITECTURES), $(shell export GOOS=$(GOOS); export GOARCH=$(GOARCH); go build $(LDFLAGS) -v -o $(BINARY)-v${VERSION}--$(GOOS)-$(GOARCH))))

update: 
	@git submodule update --init --recursive

clean:
	@$(RM) ${BINARY}
	@$(RM) concierge-protos-v*.jar
	@find ${ROOT_DIR} -name '${BINARY}[-?][a-zA-Z0-9]*[-?][a-zA-Z0-9]*' -delete
