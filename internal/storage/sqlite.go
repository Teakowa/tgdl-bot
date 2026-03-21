package storage

import (
	"context"
	"database/sql"
	"errors"
)

type SQLiteStore struct {
	DB       *sql.DB
	Migrator *MigrationRunner
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{
		DB:       db,
		Migrator: NewMigrationRunner(db),
	}
}

func (s *SQLiteStore) ApplyMigrations(ctx context.Context, migrations ...Migration) error {
	if s == nil || s.Migrator == nil {
		return errors.New("storage: nil sqlite store")
	}
	return s.Migrator.Apply(ctx, migrations...)
}

func (s *SQLiteStore) TaskRepository() *SQLiteTaskRepository {
	if s == nil {
		return nil
	}
	return &SQLiteTaskRepository{DB: s.DB}
}
