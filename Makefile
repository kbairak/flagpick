DEST ?= $(or $(XDG_CONFIG_HOME),$(HOME)/.config)/flagpick

.PHONY: all build test vet fmt install install-config manifest clean

all: fmt test vet manifest install-config build

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

manifest:
	go run ./cmd/genmanifest

clean:
	rm -f fp
