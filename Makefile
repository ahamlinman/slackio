SRC_FILES = $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: test
test: vendor
	go test -cover -timeout 10s -race .

vendor Gopkg.lock: Gopkg.toml $(SRC_FILES)
	dep ensure
	touch vendor
