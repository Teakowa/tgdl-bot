package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type D1Store struct {
	Client *D1Client
	Repo   *D1TaskRepository
}

func NewD1Store(client *D1Client) *D1Store {
	return &D1Store{
		Client: client,
		Repo:   NewD1TaskRepository(client),
	}
}

func (s *D1Store) ApplyMigrations(ctx context.Context, migrations ...Migration) error {
	if s == nil || s.Client == nil {
		return errors.New("storage: nil d1 store")
	}

	for _, migration := range migrations {
		for _, statement := range splitStatements(migration.SQL) {
			if addColumn, ok := parseConditionalAddColumnStatement(statement); ok {
				if err := s.applyConditionalAddColumn(ctx, addColumn); err != nil {
					return fmt.Errorf("storage: apply migration %q: %w", migration.Name, err)
				}
				continue
			}
			if _, err := s.Client.Query(ctx, statement); err != nil {
				return fmt.Errorf("storage: apply migration %q: %w", migration.Name, err)
			}
		}
	}
	return nil
}

func (s *D1Store) TaskRepository() *D1TaskRepository {
	if s == nil {
		return nil
	}
	return s.Repo
}

func (s *D1Store) applyConditionalAddColumn(ctx context.Context, stmt conditionalAddColumnStatement) error {
	result, err := s.Client.Query(ctx, fmt.Sprintf("PRAGMA table_info(%s)", stmt.TableName))
	if err != nil {
		return err
	}

	columnName := normalizeIdentifier(stmt.ColumnName)
	for _, row := range result.Results {
		name, ok := row["name"].(string)
		if ok && strings.EqualFold(normalizeIdentifier(name), columnName) {
			return nil
		}
	}

	_, err = s.Client.Query(ctx, stmt.AlterSQL)
	return err
}
