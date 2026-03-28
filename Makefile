.PHONY: all build test fmt vet install-binary

all: build

build:
	go build -o send-email .

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

install-binary: build
	install -d "$(DESTDIR)/opt/scripts"
	install -m 0755 send-email "$(DESTDIR)/opt/scripts/send-email"
