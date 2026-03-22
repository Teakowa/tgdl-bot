package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type Migration struct {
	Name string
	SQL  string
}

type MigrationRunner struct {
	Exec Execer
}

type conditionalAddColumnStatement struct {
	TableName  string
	ColumnName string
	AlterSQL   string
}

var alterTableAddColumnIfNotExistsPattern = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+([^\s]+)\s+ADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\s+([^\s]+)\s+(.+)$`)

func NewMigrationRunner(exec Execer) *MigrationRunner {
	return &MigrationRunner{Exec: exec}
}

func (r *MigrationRunner) Apply(ctx context.Context, migrations ...Migration) error {
	if r == nil || r.Exec == nil {
		return errors.New("storage: nil migration execer")
	}

	for _, migration := range migrations {
		for _, statement := range splitStatements(migration.SQL) {
			if _, err := r.Exec.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("storage: apply migration %q: %w", migration.Name, err)
			}
		}
	}

	return nil
}

func LoadMigrationsFromFS(fsys fs.FS, paths ...string) ([]Migration, error) {
	migrations := make([]Migration, 0, len(paths))
	for _, path := range paths {
		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("storage: read migration %q: %w", path, err)
		}
		migrations = append(migrations, Migration{
			Name: path,
			SQL:  string(content),
		})
	}
	return migrations, nil
}

func splitStatements(sqlText string) []string {
	lines := strings.Split(sqlText, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		cleaned = append(cleaned, line)
	}

	joined := strings.Join(cleaned, "\n")
	parts := strings.Split(joined, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}
	return statements
}

func parseConditionalAddColumnStatement(statement string) (conditionalAddColumnStatement, bool) {
	matches := alterTableAddColumnIfNotExistsPattern.FindStringSubmatch(strings.TrimSpace(statement))
	if len(matches) != 4 {
		return conditionalAddColumnStatement{}, false
	}

	tableName := strings.TrimSpace(matches[1])
	columnName := strings.TrimSpace(matches[2])
	columnDef := strings.TrimSpace(matches[3])
	if tableName == "" || columnName == "" || columnDef == "" {
		return conditionalAddColumnStatement{}, false
	}

	return conditionalAddColumnStatement{
		TableName:  tableName,
		ColumnName: columnName,
		AlterSQL:   fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDef),
	}, true
}

func normalizeIdentifier(name string) string {
	return strings.Trim(strings.TrimSpace(name), "\"'`[]")
}
