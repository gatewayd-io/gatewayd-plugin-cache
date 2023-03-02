package plugin

import (
	"encoding/base64"

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
		if stmt.Stmt.GetSelectStmt() != nil {
			for _, fromClause := range stmt.Stmt.GetSelectStmt().FromClause {
				tables = append(tables, fromClause.GetRangeVar().Relname)
			}
		}

		if stmt.Stmt.GetInsertStmt() != nil {
			tables = append(tables, stmt.Stmt.GetInsertStmt().Relation.Relname)
		}

		if stmt.Stmt.GetUpdateStmt() != nil {
			tables = append(tables, stmt.Stmt.GetUpdateStmt().Relation.Relname)
		}

		if stmt.Stmt.GetDeleteStmt() != nil {
			tables = append(tables, stmt.Stmt.GetDeleteStmt().Relation.Relname)
		}
	}

	return tables, nil
}
