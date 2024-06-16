<p align="center">
  <a href="https://docs.gatewayd.io/plugins/gatewayd-plugin-cache">
    <picture>
      <img alt="gatewayd-plugin-cache-logo" src="https://github.com/gatewayd-io/gatewayd-plugin-cache/blob/main/assets/gatewayd-plugin-cache-logo.png" width="96" />
    </picture>
  </a>
  <h3 align="center">gatewayd-plugin-cache</h3>
  <p align="center">GatewayD plugin for caching query results.</p>
</p>

<p align="center">
    <a href="https://github.com/gatewayd-io/gatewayd-plugin-cache/releases">Download</a> ·
    <a href="https://docs.gatewayd.io/plugins/gatewayd-plugin-cache">Documentation</a>
</p>

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
- Skip caching date-time related functions
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

## Sentry

This plugin uses [Sentry](https://sentry.io) for error tracking. Sentry can be configured using the `SENTRY_DSN` environment variable. If `SENTRY_DSN` is not set, Sentry will not be used.
