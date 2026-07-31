// Command migrate applies golang-migrate migrations embedded from
// cmd/migrate/migrations against DATABASE_URL. Run automatically as a
// one-shot service in deploy/docker-compose.yml before the API binaries
// start.
package main

import (
	"database/sql"
	"embed"
	"errors"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/alfredomendoza/questlog/backend/internal/shared"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	dbURL := shared.MustEnv("DATABASE_URL")

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("migrate: open db: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("migrate: close db: %v", cerr)
		}
	}()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("migrate: postgres driver: %v", err)
	}

	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		log.Fatalf("migrate: migrations source: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "questlog", driver)
	if err != nil {
		log.Fatalf("migrate: init: %v", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("migrate: up: %v", err)
	}

	log.Println("migrate: up to date")
}
