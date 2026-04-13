package postgres

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all pending up-migrations from the given directory.
func RunMigrations(dsn, migrationsPath string, logger *slog.Logger) error {
	m, err := migrate.New("file://"+migrationsPath, dsn)
	if err != nil {
		return fmt.Errorf("init migrations: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			logger.Warn("migrate source close error", "error", srcErr)
		}
		if dbErr != nil {
			logger.Warn("migrate database close error", "error", dbErr)
		}
	}()

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	logger.Info("migrations applied", "version", version, "dirty", dirty)
	return nil
}
