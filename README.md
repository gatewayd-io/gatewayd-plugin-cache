# gatewayd-plugin-cache

GatewayD plugin for caching query results.

## Features

- Basic caching of database responses to client queries
- Invalidate cached responses on upsert and delete (table-based)
- Periodic cache invalidation
- Support for setting expiry time on cached data
- Support for caching responses from multiple databases on multiple servers
- Support nested queries: joins, unions, multi-table selects, and the like
- Detect client's chosen database from the client's startup message
- Metrics for quantifying cache hits, misses, gets, sets and deletes
- Logging at various levels
- Configurable via environment variables

## Build for testing

To build the plugin for development and testing, run the following command:

```bash
make build-dev
```

Running the above command causes the `go mod tidy` and `go build` to run for compiling and generating the plugin binary in the current directory, named `gatewayd-plugin-cache`.
