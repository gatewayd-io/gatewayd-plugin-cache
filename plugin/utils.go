package plugin

import (
	"encoding/base64"
	"net"
	"strconv"
	"strings"

	pgQuery "github.com/pganalyze/pg_query_go/v2"
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
