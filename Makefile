BINARY := ltts
PKG    := ./...

.PHONY: all build run test vet tidy clean distclean

all: build

build:
	go build -o $(BINARY) $(PKG)

run: build
	./$(BINARY)

test:
	go test $(PKG)

vet:
	go vet $(PKG)

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)

distclean: clean
	rm -rf "$${XDG_CACHE_HOME:-$$HOME/.cache}/ltts"
