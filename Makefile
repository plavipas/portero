.PHONY: build test app install icon clean
build:
	go build -o bin/portero ./cmd/portero
test:
	go vet ./... && go test ./... && sh scripts/test-install.sh
app:
	sh scripts/build-app.sh
install:
	sh scripts/install.sh
icon:
	sh scripts/make-icon.sh
clean:
	rm -rf bin build
