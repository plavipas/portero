# Command Line Tools only: swift-testing's macro plugin is not on the default search path.
CLT_PLUGINS := /Library/Developer/CommandLineTools/usr/lib/swift/host/plugins/testing
TEST_FLAGS := $(shell [ -d $(CLT_PLUGINS) ] && echo -Xswiftc -plugin-path -Xswiftc $(CLT_PLUGINS))

.PHONY: build test release clean
build:
	swift build
test:
	swift test $(TEST_FLAGS)
release:
	swift build -c release
clean:
	rm -rf .build
