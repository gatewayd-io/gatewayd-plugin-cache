package plugin

import (
	"encoding/base64"
	"net"
	"strconv"
	"strings"

	pgQuery "github.com/pganalyze/pg_query_go/v2"
)

const (
	MinPgSQLMessageLength = 5
	AddressPortPairLength = 2
)

// GetQueryFromRequest decodes the request and returns the query.
func GetQueryFromRequest(req string) (string, error) {
	requestDecoded, err := base64.StdEncoding.DecodeString(req)
	if err != nil {
		return "", err
	}

	if len(requestDecoded) < MinPgSQLMessageLength {
		return "", nil
	}

	// The first byte is the message type.
	// The next 4 bytes are the length of the message.
	// The rest of the message is the query.
	// See https://www.postgresql.org/docs/13/protocol-message-formats.html
	// for more information.
	size := int(requestDecoded[1])<<24 + int(requestDecoded[2])<<16 + int(requestDecoded[3])<<8 + int(requestDecoded[4])
	return string(requestDecoded[MinPgSQLMessageLength:size]), nil
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

	isMulti := func(stmt *pgQuery.SelectStmt) bool {
		return stmt.GetOp() == pgQuery.SetOperation_SETOP_UNION ||
			stmt.GetOp() == pgQuery.SetOperation_SETOP_INTERSECT ||
			stmt.GetOp() == pgQuery.SetOperation_SETOP_EXCEPT
	}

	// If the query is a union, we need to get the tables from both sides.
	getMultiTable := func(stmt *pgQuery.SelectStmt) {
		// Get the tables from the left side.
		for _, fromClause := range stmt.GetLarg().FromClause {
			rangeVar := fromClause.GetRangeVar()
			if rangeVar != nil {
				tables = append(tables, rangeVar.Relname)
			}
		}

		// Get the tables from the right side.
		for _, fromClause := range stmt.GetRarg().FromClause {
			rangeVar := fromClause.GetRangeVar()
			if rangeVar != nil {
				tables = append(tables, rangeVar.Relname)
			}
		}
	}

	getSingleTable := func(fromClause []*pgQuery.Node) {
		for _, from := range fromClause {
			rangeVar := from.GetRangeVar()
			if rangeVar != nil {
				tables = append(tables, rangeVar.Relname)
			}
		}
	}

	for _, stmt := range stmt.Stmts {
		if isMulti(stmt.Stmt.GetSelectStmt()) {
			getMultiTable(stmt.Stmt.GetSelectStmt())
		}

		// Get the table from the WITH clause.
		if withClause := stmt.Stmt.GetSelectStmt().GetWithClause(); withClause != nil {
			for _, cte := range withClause.Ctes {
				selectStmt := cte.GetCommonTableExpr().Ctequery.GetSelectStmt()
				if isMulti(selectStmt) {
					getMultiTable(selectStmt)
				} else {
					getSingleTable(selectStmt.FromClause)
				}
			}
		} else {
			// Get the table from the FROM clause.
			if selectQuery := stmt.Stmt.GetSelectStmt(); selectQuery != nil {
				getSingleTable(selectQuery.FromClause)
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

// validateIP checks if an IP address is valid.
func validateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	if ip.To4() != nil || ip.To16() != nil {
		return true
	}

	return false
}

// validateAddressPort validates an address:port string.
func validateAddressPort(addressPort string) bool {
	// Split the address and port.
	data := strings.Split(strings.TrimSpace(addressPort), ":")
	if len(data) != AddressPortPairLength {
		return false
	}

	// Validate the port.
	port, err := strconv.ParseUint(data[1], 10, 16)
	if err != nil {
		return false
	}

	// Resolve the IP address, if it is a host.
	ipAddress, err := net.ResolveIPAddr("ip", data[0])
	if err != nil {
		return false
	}

	// Validate the IP address and port.
	if (validateIP(net.ParseIP(data[0])) || validateIP(ipAddress.IP)) && (port > 0 && port <= 65535) {
		return true
	}

	return false
}

// validateHostPort validates a host:port string.
// TODO: Add support for IPv6.
func validateHostPort(hostPort string) bool {
	data := strings.Split(hostPort, ":")
	if len(data) != AddressPortPairLength {
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
