.PHONY: build install clean test

BINARY = ghdeb
DESTDIR ?= /usr/local/bin
MANDIR ?= /usr/local/share/man

build:
	go build -ldflags="-s -w" -o $(BINARY) ./cmd/ghdeb/

install: build
	install -d $(DESTDIR)
	install -m 755 $(BINARY) $(DESTDIR)/$(BINARY)
	# 安装英文版 man 手册（默认）
	install -d $(MANDIR)/man1
	install -m 644 man/en_US/$(BINARY).1 $(MANDIR)/man1/$(BINARY).1
	# 安装中文版 man 手册
	install -d $(MANDIR)/zh_CN/man1
	install -m 644 man/zh_CN/$(BINARY).1 $(MANDIR)/zh_CN/man1/$(BINARY).1
	@echo "提示: 运行 'mandb' 更新 man 数据库 (可选)"

clean:
	go clean

# 交叉编译
build-all:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-amd64 ./cmd/ghdeb/
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-arm64 ./cmd/ghdeb/
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-armhf ./cmd/ghdeb/
