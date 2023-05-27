# gatewayd-plugin-cache

GatewayD plugin for caching query results. For more information, see the [docs](https://gatewayd.io/docs/plugins/gatewayd-plugin-cache) for this plugin.

## Features

- Basic caching of database responses to client queries
- Invalidate cached responses by parsing incoming queries (table-based):
  - **DML**: INSERT, UPDATE and DELETE
  - **Multi-statements**: UNION, INTERSECT and EXCEPT
  - **DDL**: TRUNCATE, DROP and ALTER
  - **WITH clause**
  - **Multiple queries** (delimited by semicolon)
- Periodic cache invalidation for invalidating stale client keys
- Support for setting expiry time on cached data
- Support for caching responses from multiple databases on multiple servers
- Detect client's chosen database from the client's startup message
- Prometheus metrics for quantifying cache hits, misses, gets, sets, deletes and scans
- Prometheus metrics for counting total RPC method calls
- Logging
- Configurable via environment variables

## Build for testing

To build the plugin for development and testing, run the following command:

```bash
make build-dev
```

Running the above command causes the `go mod tidy` and `go build` to run for compiling and generating the plugin binary in the current directory, named `gatewayd-plugin-cache`.
