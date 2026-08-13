package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		panic("DATABASE_URL is required")
	}
	connection, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		panic(err)
	}
	defer connection.Close(context.Background())
	if _, err = connection.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS schema_migrations (filename text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		panic(err)
	}
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		panic(err)
	}
	sort.Strings(files)
	for _, file := range files {
		var applied bool
		if err = connection.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename=$1)`, file).Scan(&applied); err != nil {
			panic(err)
		}
		if applied {
			fmt.Println("skipped", file)
			continue
		}
		statement, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}
		if _, err = connection.Exec(context.Background(), string(statement)); err != nil {
			panic(fmt.Errorf("%s: %w", file, err))
		}
		if _, err = connection.Exec(context.Background(), `INSERT INTO schema_migrations (filename) VALUES ($1)`, file); err != nil {
			panic(err)
		}
		fmt.Println("applied", file)
	}
}
