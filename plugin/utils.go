package plugin

import (
	"context"
	"encoding/base64"
	"net"
	"strconv"
	"strings"

	"github.com/eko/gocache/lib/v4/cache"
	sdkPlugin "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin"
	pgQuery "github.com/pganalyze/pg_query_go/v2"
	"github.com/spf13/cast"
	"google.golang.org/protobuf/types/known/structpb"
)

// GetQueryFromRequest decodes the request and returns the query.
func GetQueryFromRequest(req string) (string, error) {
	requestDecoded, err := base64.StdEncoding.DecodeString(req)
	if err != nil {
		return "", err
	}

	if len(requestDecoded) < 5 {
		return "", nil
	}

	// The first byte is the message type.
	// The next 4 bytes are the length of the message.
	// The rest of the message is the query.
	// See https://www.postgresql.org/docs/13/protocol-message-formats.html
	// for more information.
	size := int(requestDecoded[1])<<24 + int(requestDecoded[2])<<16 + int(requestDecoded[3])<<8 + int(requestDecoded[4])
	return string(requestDecoded[5:size]), nil
}

// GetTablesFromQuery returns the tables used in a query.
func GetTablesFromQuery(query string) ([]string, error) {
	stmt, err := pgQuery.Parse(query)
	if err != nil {
		return nil, err
	}

	if len(stmt.Stmts) == 0 {
		return nil, nil
	}

	tables := []string{}

	for _, stmt := range stmt.Stmts {
		if selectQuery := stmt.Stmt.GetSelectStmt(); selectQuery != nil {
			for _, fromClause := range selectQuery.FromClause {
				rangeVar := fromClause.GetRangeVar()
				if rangeVar != nil {
					tables = append(tables, rangeVar.Relname)
				}
			}
		}

		if insertQuery := stmt.Stmt.GetInsertStmt(); insertQuery != nil {
			tables = append(tables, insertQuery.Relation.Relname)
		}

		if updateQuery := stmt.Stmt.GetUpdateStmt(); updateQuery != nil {
			tables = append(tables, updateQuery.Relation.Relname)
		}

		if deleteQuery := stmt.Stmt.GetDeleteStmt(); deleteQuery != nil {
			tables = append(tables, deleteQuery.Relation.Relname)
		}
	}

	return tables, nil
}

// validateAddressPort validates an address:port string.
func validateAddressPort(addressPort string) bool {
	data := strings.Split(addressPort, ":")
	if len(data) != 2 {
		return false
	}

	port, err := strconv.ParseUint(data[1], 10, 16)
	if err != nil {
		return false
	}

	if net.ParseIP(data[0]) != nil && (port > 0 && port <= 65535) {
		return true
	}

	return false
}

// validateHostPort validates a host:port string.
func validateHostPort(hostPort string) bool {
	data := strings.Split(hostPort, ":")
	if len(data) != 2 {
		return false
	}

	port, err := strconv.ParseUint(data[1], 10, 16)
	if err != nil {
		return false
	}

	// FIXME: There is not much to validate on the host side.
	if data[0] != "" && port > 0 && port <= 65535 {
		return true
	}

	return false
}

// isBusy checks if a client address exists in cache by matching the address
// with the busy clients.
func isBusy(proxies map[string]Proxy, address string) bool {
	if proxies == nil {
		// NOTE: If the API is not running, we assume that the client is busy,
		// so that we don't accidentally make the client and the plugin unstable.
		return true
	}

	for _, name := range proxies {
		for _, client := range name.Busy {
			if client == address {
				return true
			}
		}
	}
	return false
}

func (p *Plugin) invalidateDML(query string) {
	ctx := context.Background()

	// Check if the query is a UPDATE, INSERT or DELETE.
	queryDecoded, err := base64.StdEncoding.DecodeString(query)
	if err != nil {
		p.Logger.Debug("Failed to decode query", "error", err)
	} else {
		queryMessage := cast.ToStringMapString(string(queryDecoded))
		p.Logger.Trace("Query message", "query", queryMessage)

		queryString := strings.ToUpper(queryMessage["String"])
		// TODO: Add change detection for all changes to DB, not just for the DMLs.
		// https://github.com/gatewayd-io/gatewayd-plugin-cache/issues/19
		if strings.HasPrefix(queryString, "UPDATE") ||
			strings.HasPrefix(queryString, "INSERT") ||
			strings.HasPrefix(queryString, "DELETE") {
			tables, err := GetTablesFromQuery(queryMessage["String"])

			if err != nil {
				p.Logger.Debug("Failed to get tables from query", "error", err)
			} else {
				p.Logger.Trace("Tables", "tables", tables)
				for _, table := range tables {
					// Invalidate the cache for the table.
					// TODO: This is not efficient. We should be able to invalidate the cache
					// for a specific key instead of invalidating the entire table.
					pipeline := p.RedisClient.Pipeline()
					for {
						scanResult := p.RedisClient.Scan(ctx, 0, table+":*", p.ScanCount)
						if scanResult.Err() != nil {
							CacheMissesCounter.Inc()
							p.Logger.Debug("Failed to scan keys", "error", scanResult.Err())
							break
						}

						// Per each key, delete the cache entry and the table cache key itself.
						keys, cursor := scanResult.Val()
						for _, tableKey := range keys {
							// Invalidate the cache for the table.
							cachedRespnseKey := strings.TrimPrefix(tableKey, table+":")
							pipeline.Del(ctx, cachedRespnseKey)
							// Invalidate the table cache key itself.
							pipeline.Del(ctx, tableKey)
						}

						if cursor == 0 {
							break
						}
					}

					result, err := pipeline.Exec(ctx)
					if err != nil {
						p.Logger.Debug("Failed to execute pipeline", "error", err)
					}

					for _, res := range result {
						if res.Err() != nil {
							CacheMissesCounter.Inc()
						} else {
							CacheDeletesCounter.Inc()
						}
					}
				}
			}
		}
	}
}

func (p *Plugin) getDBFromStartupMessage(
	req *structpb.Struct,
	cacheManager *cache.Cache[string],
	database string,
	client map[string]string,
) string {
	ctx := context.Background()

	// Try to get the database from the startup message, which is only sent once by the client.
	// Store the database in the cache so that we can use it for subsequent requests.
	startupMessageEncoded := cast.ToString(sdkPlugin.GetAttr(req, "startupMessage", ""))
	if startupMessageEncoded != "" {
		startupMessageBytes, err := base64.StdEncoding.DecodeString(startupMessageEncoded)
		if err != nil {
			p.Logger.Debug("Failed to decode startup message", "error", err)
		} else {
			startupMessage := cast.ToStringMap(string(startupMessageBytes))
			p.Logger.Trace("Startup message", "startupMessage", startupMessage, "client", client)
			if startupMessage != nil && client != nil {
				startupMsgParams := cast.ToStringMapString(startupMessage["Parameters"])
				if startupMsgParams != nil &&
					startupMsgParams["database"] != "" &&
					client["remote"] != "" {
					if err := cacheManager.Set(
						ctx, client["remote"], startupMsgParams["database"]); err != nil {
						CacheMissesCounter.Inc()
						p.Logger.Debug("Failed to set cache", "error", err)
					}
					CacheSetsCounter.Inc()
					p.Logger.Debug("Set the database in the cache for the current session",
						"database", database, "client", client["remote"])
					return startupMsgParams["database"]
				}
			}
		}
	}

	return database
}
