package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/esnunes/prompter/internal/db"
	"github.com/esnunes/prompter/internal/repo"
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

	if len(os.Args) < 2 {
		return fmt.Errorf("usage: prompter <github.com/owner/repo>")
	}
	repoURL := os.Args[1]

	if err := repo.ValidateURL(repoURL); err != nil {
		return err
	}

	if err := checkDependencies(); err != nil {
		return err
	}

	localPath, err := repo.EnsureCloned(ctx, repoURL)
	if err != nil {
		return fmt.Errorf("setting up repository: %w", err)
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

	_, err = queries.UpsertRepository(repoURL, localPath)
	if err != nil {
		return fmt.Errorf("registering repository: %w", err)
	}

	srv, err := server.New(queries)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	return srv.Start(ctx)
}

func checkDependencies() error {
	for _, dep := range []struct {
		name    string
		helpURL string
	}{
		{"claude", "https://docs.anthropic.com/en/docs/claude-code"},
		{"gh", "https://cli.github.com"},
	} {
		if _, err := findExecutable(dep.name); err != nil {
			return fmt.Errorf("%s CLI not found. Install: %s", dep.name, dep.helpURL)
		}
	}
	return nil
}

func findExecutable(name string) (string, error) {
	return exec.LookPath(name)
}
