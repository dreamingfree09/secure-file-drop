package server

import "database/sql"

func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		Warn("database rows close failed", nil)
	}
}
