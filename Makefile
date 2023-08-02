PROJECT_URL=github.com/gatewayd-io/gatewayd
CONFIG_PACKAGE=${PROJECT_URL}/config
LAST_TAGGED_COMMIT=$(shell git rev-list --tags --max-count=1)
VERSION=$(shell git describe --tags ${LAST_TAGGED_COMMIT})
TIMESTAMP=$(shell date -u +"%FT%T%z")
VERSION_DETAILS=${TIMESTAMP}/${LAST_TAGGED_COMMIT_SHORT}
EXTRA_LDFLAGS=-X ${CONFIG_PACKAGE}.Version=${VERSION} -X ${CONFIG_PACKAGE}.VersionDetails=${VERSION_DETAILS}
FILES=gatewayd-plugin-cache checksum.txt

tidy:
	@go mod tidy

test:
	@go test -v ./...

checksum:
	@sha256sum -b gatewayd-plugin-cache

update-all:
	@go get -u ./...

build-dev: tidy
	@go build

create-build-dir:
	@mkdir -p dist

build-linux-amd64: tidy
	@echo "Building gatewayd ${VERSION} for linux-amd64"
	@mkdir -p dist/linux-amd64
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS}" -o dist/linux-amd64/gatewayd-plugin-cache
	@sha256sum dist/linux-amd64/gatewayd-plugin-cache | sed 's/dist\/linux-amd64\///g' >> dist/linux-amd64/checksum.txt
	@tar czf dist/gatewayd-plugin-cache-linux-amd64-${VERSION}.tar.gz -C ./dist/linux-amd64/ ${FILES}
	@sha256sum dist/gatewayd-plugin-cache-linux-amd64-${VERSION}.tar.gz | sed 's/dist\///g' >> dist/checksums.txt

build-linux-arm64:
	@echo "Building gatewayd ${VERSION} for linux-arm64"
	@mkdir -p dist/linux-arm64
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS}" -o dist/linux-arm64/gatewayd-plugin-cache
	@sha256sum dist/linux-arm64/gatewayd-plugin-cache | sed 's/dist\/linux-arm64\///g' >> dist/linux-arm64/checksum.txt
	@tar czf dist/gatewayd-plugin-cache-linux-arm64-${VERSION}.tar.gz -C ./dist/linux-arm64/ ${FILES}
	@sha256sum dist/gatewayd-plugin-cache-linux-arm64-${VERSION}.tar.gz | sed 's/dist\///g' >> dist/checksums.txt

build-release: tidy create-build-dir build-linux-amd64 build-linux-arm64
