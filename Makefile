DEST ?= $(or $(XDG_CONFIG_HOME),$(HOME)/.config)/flagpick

.PHONY: all build test vet lint fmt install install-config uninstall-config manifest clean

all: fmt test vet manifest install-config build

build:
	go build -o fp .

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -l -w .

install:
	go install .

install-config:
	mkdir -p $(DEST)
	cp config/*.yaml $(DEST)/

# Remove the override configs written by 'install-config' so that flagpick
# falls back to the configs downloaded by 'fp --update'. Only removes files
# that also live in config/; any configs you authored yourself are kept.
uninstall-config:
	rm -f $(addprefix $(DEST)/,$(notdir $(wildcard config/*.yaml)))

manifest:
	go run ./cmd/genmanifest

clean:
	rm -f fp
