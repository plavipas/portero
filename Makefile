.PHONY: build test app package install icon clean
build:
	go build -o bin/portero ./cmd/portero
test:
	go vet ./... && go test ./... && sh scripts/test-install.sh
app:
	sh scripts/build-app.sh
package:
	sh scripts/package.sh
install:
	sh scripts/install.sh
icon:
	sh scripts/make-icon.sh
clean:
	rm -rf bin build
