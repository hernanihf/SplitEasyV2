package config

import (
	"fmt"
	"log"
	"os"

	"spliteasy/internal/domain"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// ConnectDB initializes the database connection
func ConnectDB() {
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

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Connected to PostgreSQL database!")

	// Run Auto migrations
	err = DB.AutoMigrate(&domain.User{}, &domain.Group{}, &domain.Expense{}, &domain.ExpenseSplit{}, &domain.ExpenseItem{}, &domain.Settlement{})
	if err != nil {
		log.Fatalf("Failed to auto-migrate database: %v", err)
	}

	log.Println("Database migration completed!")
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
		log.Fatalf("%s environment variable is required but not set", key)
	}
	return value
}
