
ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
BINARY=concierge
VERSION=0.1
COMMIT=`git rev-list HEAD --max-count=1 --abbrev-commit`
COMMIT_DATE=`git log -1 --format=%cd --date=format:'%Y%m%d%H%M%S'`
PLATFORMS=darwin linux windows
ARCHITECTURES=amd64
GOOGLEAPIS=${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT}"

default: generate test build

all: generate test build build_all

generate:
	mkdir -p apiv1
	protoc -Iprotos/concierge-api/v1 -I${GOOGLEAPIS} --go_out=plugins=grpc:${GOPATH}/src model.proto
	protoc -Iprotos/concierge-api/v1 -I${GOOGLEAPIS} --go_out=plugins=grpc:${GOPATH}/src services.proto

	# when the gRPC team release a standalone protoc-gen-go-grpc binary, use that like this, 
	#protoc -Iprotos/concierge-api/v1 -I${GOOGLEAPIS} --go_out=${GOPATH}/src --go-grpc_out=${GOPATH}/src model.proto
	#protoc -Iprotos/concierge-api/v1 -I${GOOGLEAPIS} --go_out=${GOPATH}/src --go-grpc_out=${GOPATH}/src empi.proto
	protoc -Iprotos/concierge-api/v1 -I${GOOGLEAPIS} --grpc-gateway_out=logtostderr=true:${GOPATH}/src services.proto
#	protoc -Ivendor/concierge-api/v1 -I${GOOGLEAPIS} --swagger_out=logtostderr=true:. services.proto

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
	@go build $(LDFLAGS) -o ${BINARY}-${VERSION}-${COMMIT_DATE}-${COMMIT}

build_all:
	$(foreach GOOS, $(PLATFORMS),\
	$(foreach GOARCH, $(ARCHITECTURES), $(shell export GOOS=$(GOOS); export GOARCH=$(GOARCH); go build $(LDFLAGS) -v -o $(BINARY)-v${VERSION}--$(GOOS)-$(GOARCH))))

update: 
	@git submodule update --init --recursive

clean:
	@$(RM) ${BINARY}
	@$(RM) concierge-protos-v*.jar
	@find ${ROOT_DIR} -name '${BINARY}[-?][a-zA-Z0-9]*[-?][a-zA-Z0-9]*' -delete
