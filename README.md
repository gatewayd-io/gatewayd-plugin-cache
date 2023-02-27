# gatewayd-plugin-cache

GatewayD plugin for caching query results.

## Features

- Basic caching of database responses to client queries
- Support for setting expiry time on cached data
- Support for caching responses from multiple databases on multiple servers
- Detect client's chosen database from the client's startup message
- Metrics for quantifying cache hits, misses, gets, sets and deletes
- Logging at various levels
- Configurable via environment variables
