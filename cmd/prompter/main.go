package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"

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

	if err := srv.Listen(); err != nil {
		return err
	}

	openBrowser("http://" + srv.Addr())

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

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}
