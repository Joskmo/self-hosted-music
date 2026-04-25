package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func New(ctx context.Context) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		env("DB_HOST", "auth-db"),
		env("DB_PORT", "5432"),
		env("DB_USER", "auth"),
		os.Getenv("DB_PASSWORD"),
		env("DB_NAME", "auth"),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS invites (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			code TEXT UNIQUE NOT NULL,
			used BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username TEXT UNIQUE NOT NULL,
			name TEXT,
			is_admin BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT NOW()
		);`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS navidrome_id TEXT;`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_navidrome_id ON users(navidrome_id);`,
	}
	for _, q := range migrations {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}
	}

	return db, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
