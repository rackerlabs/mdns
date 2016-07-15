MDNS_MYSQL_TAG=mdns-mysql
MYSQL_CID=$(shell docker ps | grep "$(MDNS_MYSQL_TAG) " | cut -f1 -d' ')
SOURCEDIR=.
SOURCES := $(shell find $(SOURCEDIR) -path ./docker -prune -o -name '*.go')
PKG_NAME=mdns

help:
		@echo ""
		@echo "build          - builds a mdns executable"
		@echo "test           - runs tests"
		@echo "run            - runs mdns"
		@echo "clean          - cleans up built binaries"
		@echo ""

build: fmt
		GO15VENDOREXPERIMENT=1 go build -o mdns -ldflags "-X main.builddate=`date -u '+%Y-%m-%d_%I:%M:%S%p'` -X main.gitref=`git rev-parse HEAD`" cmd/mdns.go

build-docker: $(SOURCES)
	docker run --rm -v `pwd`:/go/src/$(PKG_NAME) -w /go/src/$(PKG_NAME) golang:1.6 make build

test-docker-build:
		cd test_resources && docker build -t $(MDNS_MYSQL_TAG) -f mysql.Dockerfile .

test-docker-run:
		docker run --name $(MDNS_MYSQL_TAG) -d -p 127.0.0.1:3306:3306 -t $(MDNS_MYSQL_TAG)
		@echo "Waiting for mysql to come up"
		sleep 10

test-docker-kill%:
		docker rm -f $(MYSQL_CID) || true

test: test-docker-build test-docker-kill1 test-docker-run runtests test-docker-kill2

runtests:
		GO15VENDOREXPERIMENT=1 go test -v -coverprofile cover.out -bench=.
		GO15VENDOREXPERIMENT=1 go tool cover -func=cover.out

run:
		./mdns -debug

fmt:
		find . -maxdepth 2 -name '*.go' -exec go fmt '{}' \;

clean: test-docker-kill1
		rm -rf mdns
