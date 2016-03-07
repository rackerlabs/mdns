help:
		@echo ""
		@echo "build          - builds a mdns executable"
		@echo "test           - runs tests"
		@echo "run            - runs mdns"
		@echo "clean          - cleans up built binaries"
		@echo ""

build: fmt
		GO15VENDOREXPERIMENT=1 go build -o mdns -ldflags "-X main.builddate=`date -u '+%Y-%m-%d_%I:%M:%S%p'` -X main.gitref=`git rev-parse HEAD`" main.go

run:
		./mdns -debug

fmt:
		find . -maxdepth 2 -name '*.go' -exec go fmt '{}' \;

clean:
		rm -rf mdns

