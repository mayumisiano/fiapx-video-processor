// Package migrate applies a service's own embedded schema migrations on
// startup, replacing a separate init-container/job pattern (docs/adr/0009).
package migrate

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Run applies every pending migration in fs against databaseURL. Safe to
// call from more than one replica at once (e.g. video-api and
// video-worker, or a scaled video-worker): the postgres driver takes a
// session-level advisory lock, so concurrent callers serialize instead of
// racing on the same schema — the loser(s) simply see ErrNoChange.
func Run(databaseURL string, fs embed.FS) error {
	src, err := iofs.New(fs, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
