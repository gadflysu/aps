BINARY := aps
PREFIX ?= $(HOME)/.local/bin

.PHONY: build install clean

build:
	go build -o $(BINARY) .

install: build
	install -d $(PREFIX)
	install -m 0755 $(BINARY) $(PREFIX)/$(BINARY)
	@echo "Installed $(PREFIX)/$(BINARY)"

clean:
	rm -f $(BINARY)
