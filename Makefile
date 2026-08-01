.PHONY: build install clean test

BINARY = ghdeb
DESTDIR ?= /usr/local/bin

build:
	go build -ldflags="-s -w" -o $(BINARY) ./cmd/ghdeb/

install: build
	install -m 755 $(BINARY) $(DESTDIR)/$(BINARY)

clean:
	rm -f $(BINARY)
	go clean

# 交叉编译
build-all:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-amd64 ./cmd/ghdeb/
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-arm64 ./cmd/ghdeb/
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-armhf ./cmd/ghdeb/
