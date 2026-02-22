.PHONY: build test clean

build:
	go build -o claw ./cmd/claw

test:
	go test -vet=off -v ./internal/...

clean:
	rm -f claw
