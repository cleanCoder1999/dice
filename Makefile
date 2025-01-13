.DEFAULT := build


.PHONY: fmt, vet, build, clean, generate, staticcheck

fmt:
	go fmt ./...

vet: fmt
	go vet ./...

build: vet
	go build

clean:
	go clean

generate:
	go generate

staticcheck:
	staticcheck ./...
