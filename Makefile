tidy:
	go mod tidy

build: tidy
	go build -ldflags "-s -w"

test:
	go test -v ./...

checksum:
	sha256sum -b gatewayd-plugin-cache

update-all:
	go get -u ./...
