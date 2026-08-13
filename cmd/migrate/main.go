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
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		panic(err)
	}
	sort.Strings(files)
	for _, file := range files {
		statement, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}
		if _, err = connection.Exec(context.Background(), string(statement)); err != nil {
			panic(fmt.Errorf("%s: %w", file, err))
		}
		fmt.Println("applied", file)
	}
}
