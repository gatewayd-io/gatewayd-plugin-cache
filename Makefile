PLUGIN_NAME=gatewayd-plugin-cache
PROJECT_URL=github.com/gatewayd-io/gatewayd
CONFIG_PACKAGE=${PROJECT_URL}/config
LAST_TAGGED_COMMIT=$(shell git rev-list --tags --max-count=1)
VERSION=$(shell git describe --tags ${LAST_TAGGED_COMMIT})
TIMESTAMP=$(shell date -u +"%FT%T%z")
VERSION_DETAILS=${TIMESTAMP}/${LAST_TAGGED_COMMIT_SHORT}
EXTRA_LDFLAGS=-X ${CONFIG_PACKAGE}.Version=${VERSION} -X ${CONFIG_PACKAGE}.VersionDetails=${VERSION_DETAILS}
FILES=gatewayd-plugin-cache checksum.txt gatewayd_plugin.yaml README.md LICENSE

tidy:
	@go mod tidy

test:
	@go test -v ./...

checksum:
	@sha256sum -b gatewayd-plugin-cache

update-all:
	@go get -u ./...

build-dev: tidy
	@CGO_ENABLED=0 go build

create-build-dir:
	@mkdir -p dist

build-release: tidy create-build-dir
	@echo "Building gatewayd ${VERSION} for release"
	@$(MAKE) build-platform GOOS=linux GOARCH=amd64 OUTPUT_DIR=dist/linux-amd64
	@$(MAKE) build-platform GOOS=linux GOARCH=arm64 OUTPUT_DIR=dist/linux-arm64
	@$(MAKE) build-platform GOOS=darwin GOARCH=amd64 OUTPUT_DIR=dist/darwin-amd64
	@$(MAKE) build-platform GOOS=darwin GOARCH=arm64 OUTPUT_DIR=dist/darwin-arm64
	@$(MAKE) build-platform GOOS=windows GOARCH=amd64 OUTPUT_DIR=dist/windows-amd64
	@$(MAKE) build-platform GOOS=windows GOARCH=arm64 OUTPUT_DIR=dist/windows-arm64

build-platform: tidy
	@echo "Building gatewayd ${VERSION} for $(GOOS)-$(GOARCH)"
	@mkdir -p $(OUTPUT_DIR)
	@cp README.md LICENSE gatewayd_plugin.yaml $(OUTPUT_DIR)/
	@GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS}" -o $(OUTPUT_DIR)/$(PLUGIN_NAME)
	@sha256sum $(OUTPUT_DIR)/$(PLUGIN_NAME) | sed 's#$(OUTPUT_DIR)/##g' >> $(OUTPUT_DIR)/checksum.txt
	@if [ "$(GOOS)" = "windows" ]; then \
		zip -q -r dist/$(PLUGIN_NAME)-$(GOOS)-$(GOARCH)-${VERSION}.zip -j $(OUTPUT_DIR)/; \
	else \
		tar czf dist/$(PLUGIN_NAME)-$(GOOS)-$(GOARCH)-${VERSION}.tar.gz -C $(OUTPUT_DIR)/ ${FILES}; \
	fi
	@sha256sum dist/$(PLUGIN_NAME)-$(GOOS)-$(GOARCH)-${VERSION}.* | sed '#dist/##g' >> dist/checksums.txt
