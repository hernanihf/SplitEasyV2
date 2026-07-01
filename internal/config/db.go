package config

import (
	"fmt"
	"log/slog"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ConnectDB opens the database connection and applies pending migrations.
// It returns the *gorm.DB rather than storing it in a package variable, so
// callers wire it through dependency injection (as the repositories already
// do) instead of every package being able to reach into a global.
func ConnectDB() (*gorm.DB, error) {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "postgres")
	dbname := getEnv("DB_NAME", "spliteasy")
	// Secure by default: TLS is required unless explicitly disabled (e.g. a local
	// Postgres without SSL sets DB_SSLMODE=disable).
	sslmode := getEnv("DB_SSLMODE", "require")

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
		host, user, password, dbname, port, sslmode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	slog.Info("connected to PostgreSQL database")

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	if err := RunMigrations(sqlDB); err != nil {
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	return db, nil
}

// getEnv gets an environment variable or returns a fallback
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// mustGetEnv gets a required environment variable, or fails startup if it's
// missing/empty. Use this for secrets that must never silently fall back to
// a hardcoded default (e.g. JWT_SECRET) — a missing value should stop the
// process, not boot it with a value anyone reading the source code knows.
func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return value
}
