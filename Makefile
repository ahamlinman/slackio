SRC_FILES = $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: test
test:
	go test -v -mod=readonly -cover -timeout 10s -race .
