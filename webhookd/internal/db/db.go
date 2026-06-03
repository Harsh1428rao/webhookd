package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func Connect() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		host := os.Getenv("DB_HOST")
		port := os.Getenv("DB_PORT")
		user := os.Getenv("DB_USER")
		password := os.Getenv("DB_PASSWORD")
		dbname := os.Getenv("DB_NAME")
		if port == "" {
			port = "5432"
		}
		dsn = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			host, port, user, password, dbname,
		)
	}

	database, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("could not ping database: %w", err)
	}
	database.SetMaxOpenConns(25)
	database.SetMaxIdleConns(5)
	return database, nil
}

func RunMigrations(database *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id         SERIAL PRIMARY KEY,
			email      TEXT UNIQUE NOT NULL,
			password   TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS endpoints (
			id         SERIAL PRIMARY KEY,
			user_id    INTEGER REFERENCES users(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			url        TEXT NOT NULL,
			secret     TEXT,
			is_active  BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS deliveries (
			id               SERIAL PRIMARY KEY,
			endpoint_id      INTEGER REFERENCES endpoints(id) ON DELETE CASCADE,
			payload          TEXT NOT NULL,
			event_type       TEXT NOT NULL DEFAULT 'generic',
			status           TEXT NOT NULL DEFAULT 'pending',
			attempt_count    INTEGER DEFAULT 0,
			max_attempts     INTEGER DEFAULT 5,
			next_retry_at    TIMESTAMPTZ,
			last_status_code INTEGER,
			last_error       TEXT,
			delivered_at     TIMESTAMPTZ,
			created_at       TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS delivery_attempts (
			id           SERIAL PRIMARY KEY,
			delivery_id  INTEGER REFERENCES deliveries(id) ON DELETE CASCADE,
			status_code  INTEGER,
			error        TEXT,
			duration_ms  INTEGER,
			attempted_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_deliveries_status       ON deliveries(status)`,
		`CREATE INDEX IF NOT EXISTS idx_deliveries_next_retry   ON deliveries(next_retry_at)`,
		`CREATE INDEX IF NOT EXISTS idx_deliveries_endpoint_id  ON deliveries(endpoint_id)`,
		`CREATE INDEX IF NOT EXISTS idx_endpoints_user_id       ON endpoints(user_id)`,
	}

	for _, q := range queries {
		if _, err := database.Exec(q); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}
