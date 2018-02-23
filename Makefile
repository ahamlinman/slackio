SRC_FILES = $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: test
test: .deps-installed
	go test -cover -timeout 10s -race .

.deps-installed: $(SRC_FILES)
	go get ./...
	touch .deps-installed
