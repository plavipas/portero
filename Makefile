.PHONY: build test app install clean
build:
	go build -o bin/portero ./cmd/portero
test:
	go vet ./... && go test ./...
app:
	sh scripts/build-app.sh
install:
	sh scripts/install.sh
clean:
	rm -rf bin build
