# gatewayd-plugin-cache

GatewayD plugin for caching query results.

## Features

- Basic caching of database responses to client queries
- Invalidate cached responses on upsert and delete (table-based)
- Support for setting expiry time on cached data
- Support for caching responses from multiple databases on multiple servers
- Detect client's chosen database from the client's startup message
- Metrics for quantifying cache hits, misses, gets, sets and deletes
- Logging at various levels
- Configurable via environment variables

## Build

To build the plugin, run the following command:

```bash
make && make checksum
```

Running the above command causes these command to run:

1. `go mod tidy && go build -ldflags "-s -w"` ⇒ compiles and generates `gatewayd-plugin-cache`.
2. `sha256sum -b gatewayd-plugin-cache` ⇒ generates SHA256 hash.

For now, the generated hash should be manually replaced with the old one in [gatewayd_plugins.yaml](https://github.com/gatewayd-io/gatewayd/blob/1e06a1d9f1e8a9f455992cbf43fedf587a92a81e/gatewayd_plugins.yaml#L73).
