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

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS invites (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			code TEXT UNIQUE NOT NULL,
			used BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username TEXT UNIQUE NOT NULL,
			name TEXT,
			is_admin BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT NOW()
		);
	`); err != nil {
		return nil, fmt.Errorf("create tables: %w", err)
	}

	return db, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
