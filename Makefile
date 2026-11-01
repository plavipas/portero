.PHONY: build test vet clean
build:
	go build -o bin/portero ./cmd/portero
test:
	go vet ./... && go test ./...
clean:
	rm -rf bin
