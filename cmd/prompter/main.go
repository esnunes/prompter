package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/github"
	"github.com/esnunes/prompter/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := checkDependencies(ctx); err != nil {
		return err
	}

	dbPath, err := db.DBPath()
	if err != nil {
		return err
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	queries := db.NewQueries(database)

	srv, err := server.New(queries)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	host := os.Getenv("PROMPTER_HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := os.Getenv("PROMPTER_PORT")
	if port == "" {
		port = "8080"
	}

	if err := srv.Listen(net.JoinHostPort(host, port)); err != nil {
		return err
	}

	return srv.Serve(ctx)
}

func checkDependencies(ctx context.Context) error {
	for _, dep := range []struct {
		name    string
		helpURL string
	}{
		{"claude", "https://docs.anthropic.com/en/docs/claude-code"},
		{"gh", "https://cli.github.com"},
	} {
		if _, err := exec.LookPath(dep.name); err != nil {
			return fmt.Errorf("%s CLI not found. Install: %s", dep.name, dep.helpURL)
		}
	}

	// Check gh authentication
	if err := github.CheckAuth(ctx); err != nil {
		return err
	}

	return nil
}
