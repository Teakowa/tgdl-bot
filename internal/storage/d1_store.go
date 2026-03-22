package storage

import (
	"context"
	"errors"
	"fmt"
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
