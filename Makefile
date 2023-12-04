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

build-linux-amd64: tidy
	@echo "Building gatewayd ${VERSION} for linux-amd64"
	@mkdir -p dist/linux-amd64
	@cp README.md LICENSE gatewayd_plugin.yaml ./dist/linux-amd64/
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS}" -o dist/linux-amd64/gatewayd-plugin-cache
	@sha256sum dist/linux-amd64/gatewayd-plugin-cache | sed 's/dist\/linux-amd64\///g' >> dist/linux-amd64/checksum.txt
	@tar czf dist/gatewayd-plugin-cache-linux-amd64-${VERSION}.tar.gz -C ./dist/linux-amd64/ ${FILES}
	@sha256sum dist/gatewayd-plugin-cache-linux-amd64-${VERSION}.tar.gz | sed 's/dist\///g' >> dist/checksums.txt

build-linux-arm64:
	@echo "Building gatewayd ${VERSION} for linux-arm64"
	@mkdir -p dist/linux-arm64
	@cp README.md LICENSE gatewayd_plugin.yaml ./dist/linux-arm64/
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 CC=aarch64-linux-gnu-gcc go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS}" -o dist/linux-arm64/gatewayd-plugin-cache
	@sha256sum dist/linux-arm64/gatewayd-plugin-cache | sed 's/dist\/linux-arm64\///g' >> dist/linux-arm64/checksum.txt
	@tar czf dist/gatewayd-plugin-cache-linux-arm64-${VERSION}.tar.gz -C ./dist/linux-arm64/ ${FILES}
	@sha256sum dist/gatewayd-plugin-cache-linux-arm64-${VERSION}.tar.gz | sed 's/dist\///g' >> dist/checksums.txt

build-darwin-amd64:
	@echo "Building gatewayd ${VERSION} for darwin-arm64"
	@mkdir -p dist/darwin-amd64
	@cp README.md LICENSE gatewayd_plugin.yaml ./dist/darwin-amd64/
	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS}" -o dist/darwin-amd64/gatewayd-plugin-cache
	@shasum -a 256 dist/darwin-amd64/gatewayd-plugin-cache | sed 's/dist\/darwin-amd64\///g' >> dist/darwin-amd64/checksum.txt
	@tar czf dist/gatewayd-plugin-cache-darwin-amd64-${VERSION}.tar.gz -C ./dist/darwin-amd64/ ${FILES}
	@shasum -a 256 dist/gatewayd-plugin-cache-darwin-amd64-${VERSION}.tar.gz | sed 's/dist\///g' >> dist/checksums.txt

build-darwin-arm64:
	@echo "Building gatewayd ${VERSION} for darwin-arm64"
	@mkdir -p dist/darwin-arm64
	@cp README.md LICENSE gatewayd_plugin.yaml ./dist/darwin-arm64/
	@GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS}" -o dist/darwin-arm64/gatewayd-plugin-cache
	@shasum -a 256 dist/darwin-arm64/gatewayd-plugin-cache | sed 's/dist\/darwin-arm64\///g' >> dist/darwin-arm64/checksum.txt
	@tar czf dist/gatewayd-plugin-cache-darwin-arm64-${VERSION}.tar.gz -C ./dist/darwin-arm64/ ${FILES}
	@shasum -a 256 dist/gatewayd-plugin-cache-darwin-arm64-${VERSION}.tar.gz | sed 's/dist\///g' >> dist/checksums.txt

build-windows-amd64:
	@echo "Building gatewayd ${VERSION} for windows-amd64"
	@mkdir -p dist/windows-amd64
	@cp README.md LICENSE gatewayd_plugin.yaml ./dist/windows-amd64/
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS}" -o dist/windows-amd64/gatewayd-plugin-cache.exe
	@sha256sum dist/windows-amd64/gatewayd-plugin-cache.exe | sed 's/dist\/windows-amd64\///g' >> dist/windows-amd64/checksum.txt
	@zip -r dist/gatewayd-plugin-cache-windows-amd64-${VERSION}.zip -j ./dist/windows-amd64/
	@sha256sum dist/gatewayd-plugin-cache-windows-amd64-${VERSION}.zip | sed 's/dist\///g' >> dist/checksums.txt

build-windows-arm64:
	@echo "Building gatewayd ${VERSION} for windows-arm64"
	@mkdir -p dist/windows-arm64
	@cp README.md LICENSE gatewayd_plugin.yaml ./dist/windows-arm64/
	@GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS}" -o dist/windows-arm64/gatewayd-plugin-cache.exe
	@sha256sum dist/windows-arm64/gatewayd-plugin-cache.exe | sed 's/dist\/windows-arm64\///g' >> dist/windows-arm64/checksum.txt
	@zip -r dist/gatewayd-plugin-cache-windows-arm64-${VERSION}.zip -j ./dist/windows-arm64/
	@sha256sum dist/gatewayd-plugin-cache-windows-arm64-${VERSION}.zip | sed 's/dist\///g' >> dist/checksums.txt

build-release: tidy create-build-dir build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 build-windows-arm64

generate-release-checksums:
	@sha256sum gatewayd-plugin-cache-linux-amd64-${VERSION}.tar.gz > checksums.txt
	@sha256sum gatewayd-plugin-cache-linux-arm64-${VERSION}.tar.gz >> checksums.txt
	@sha256sum gatewayd-plugin-cache-darwin-amd64-${VERSION}.tar.gz >> checksums.txt
	@sha256sum gatewayd-plugin-cache-darwin-arm64-${VERSION}.tar.gz >> checksums.txt
	@sha256sum gatewayd-plugin-cache-windows-amd64-${VERSION}.zip >> checksums.txt
	@sha256sum gatewayd-plugin-cache-windows-arm64-${VERSION}.zip >> checksums.txt
