package config

import (
	"database/sql"
	"embed"
	"errors"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// RunMigrations applies any pending versioned migrations, replacing GORM's
// AutoMigrate. AutoMigrate only ever adds tables/columns/indexes — it can't
// express a rename or a drop, so a model change that renames a field leaves
// the old column behind instead of migrating it, and it runs unconditionally
// on every boot with no review step. Versioned SQL files under ./migrations
// make schema changes explicit and reviewable before they touch production.
func RunMigrations(sqlDB *sql.DB) error {
	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return err
	}

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			slog.Info("no pending database migrations")
			return nil
		}
		return err
	}

	slog.Info("database migrations applied")
	return nil
}
