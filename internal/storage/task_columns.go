package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type taskColumn struct {
	Name string
	Type string
}

var requiredTaskColumns = []taskColumn{
	{Name: "source_message_id", Type: "INTEGER"},
	{Name: "status_message_id", Type: "INTEGER"},
}

func EnsureTaskColumns(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("storage: nil sqlite db")
	}

	existing, err := taskColumnNames(ctx, db)
	if err != nil {
		return err
	}

	for _, column := range requiredTaskColumns {
		if _, ok := existing[column.Name]; ok {
			continue
		}

		stmt := fmt.Sprintf("ALTER TABLE tasks ADD COLUMN %s %s", column.Name, column.Type)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			// Bot and downloader may race on startup against the same SQLite file.
			if isDuplicateColumnError(err) {
				continue
			}
			return fmt.Errorf("storage: add tasks column %q: %w", column.Name, err)
		}
	}

	return nil
}

func taskColumnNames(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(tasks)")
	if err != nil {
		return nil, fmt.Errorf("storage: query tasks table info: %w", err)
	}
	defer rows.Close()

	names := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			colType    string
			notNull    int
			defaultV   sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &primaryKey); err != nil {
			return nil, fmt.Errorf("storage: scan tasks table info: %w", err)
		}
		names[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate tasks table info: %w", err)
	}
	return names, nil
}

func isDuplicateColumnError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
}
