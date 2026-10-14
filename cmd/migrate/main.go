package main

import (
	"context"
	"fmt"
	"os"

	"eventflow/cmd/config"
	"eventflow/migrations"
	"eventflow/pkg/database"
	"eventflow/pkg/migrate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "eventflow: ejecutar migraciones fallo")
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.RequestTimeout)
	defer cancel()
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		Database: cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
	})
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := migrate.Apply(ctx, migrations.Files, db)
	if err != nil {
		return err
	}
	fmt.Printf("migraciones aplicadas=%d omitidas=%d\n", result.Applied, result.Skipped)
	return nil
}
