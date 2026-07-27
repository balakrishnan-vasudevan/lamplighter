BINARY := lamplighter

.PHONY: build test clean install

build:
	go build -o $(BINARY) .

test:
	go test ./...

clean:
	rm -f $(BINARY)

install:
	go install .
