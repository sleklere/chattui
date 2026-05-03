// Package main is the entry point for the chattui client.
package main

import (
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/sleklere/chattui/cmd/client/internal/config"
	"github.com/sleklere/chattui/cmd/client/internal/ui"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	theme.Load(cfg.Theme)

	logFile, err := os.OpenFile("client.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		slog.Error("failed to open log file", "error", err)
		os.Exit(1)
	}
	defer func() {
		_ = logFile.Close()
	}()

	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}))

	app := ui.NewApp(cfg, logger)
	p := tea.NewProgram(&app, tea.WithAltScreen())
	app.SetProgram(p)

	if _, err := p.Run(); err != nil {
		logger.Error("program exited with error", "error", err)
		os.Exit(1)
	}
}
