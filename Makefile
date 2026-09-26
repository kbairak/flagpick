DEST ?= $(HOME)/.cache/flagpick

.PHONY: all build test vet fmt install install-config clean

all: fmt test vet install-config build

build:
	go build -o fp .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

install:
	go install .

install-config:
	mkdir -p $(DEST)
	cp config/*.yaml $(DEST)/

clean:
	rm -f fp
