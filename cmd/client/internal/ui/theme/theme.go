// Package theme provides color palettes for the chat client TUI.
package theme

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme defines the color palette used across all UI components.
type Theme struct {
	Name string

	// Base surface colors
	Base    lipgloss.Color // main background
	Surface lipgloss.Color // raised surfaces (cards, panels)
	Overlay lipgloss.Color // overlays, popups

	// Text colors
	Text   lipgloss.Color // primary text
	Subtle lipgloss.Color // muted/secondary text

	// Accent colors
	Accent lipgloss.Color // primary accent (highlights, borders)
	Second lipgloss.Color // secondary accent
	Gold   lipgloss.Color // warm accent (timestamps, indicators)

	// Semantic colors
	Error   lipgloss.Color // errors
	Success lipgloss.Color // success indicators

	// Message colors
	OwnMsg   lipgloss.Color // own messages highlight
	OtherMsg lipgloss.Color // other users' names
}

// Current is the active theme. Set via SetTheme.
var Current = CatppuccinMocha()

// SetTheme sets the active theme by name. Falls back to catppuccin.
func SetTheme(name string) {
	switch name {
	case "rose-pine", "rosepine":
		Current = RosePine()
	case "kanagawa", "kanagawa-dragon":
		Current = KanagawaDragon()
	default:
		Current = CatppuccinMocha()
	}
}

// CatppuccinMocha returns the Catppuccin Mocha color palette.
func CatppuccinMocha() Theme {
	return Theme{
		Name:     "catppuccin",
		Base:     lipgloss.Color("#1e1e2e"),
		Surface:  lipgloss.Color("#313244"),
		Overlay:  lipgloss.Color("#45475a"),
		Text:     lipgloss.Color("#cdd6f4"),
		Subtle:   lipgloss.Color("#6c7086"),
		Accent:   lipgloss.Color("#cba6f7"), // mauve
		Second:   lipgloss.Color("#89b4fa"), // blue
		Gold:     lipgloss.Color("#f9e2af"), // yellow
		Error:    lipgloss.Color("#f38ba8"), // red
		Success:  lipgloss.Color("#a6e3a1"), // green
		OwnMsg:   lipgloss.Color("#94e2d5"), // teal
		OtherMsg: lipgloss.Color("#89dceb"), // sky
	}
}

// RosePine returns the Rosé Pine color palette.
func RosePine() Theme {
	return Theme{
		Name:     "rose-pine",
		Base:     lipgloss.Color("#191724"),
		Surface:  lipgloss.Color("#1f1d2e"),
		Overlay:  lipgloss.Color("#26233a"),
		Text:     lipgloss.Color("#e0def4"),
		Subtle:   lipgloss.Color("#6e6a86"),
		Accent:   lipgloss.Color("#c4a7e7"), // iris
		Second:   lipgloss.Color("#9ccfd8"), // foam
		Gold:     lipgloss.Color("#f6c177"), // gold
		Error:    lipgloss.Color("#eb6f92"), // love
		Success:  lipgloss.Color("#9ccfd8"), // foam
		OwnMsg:   lipgloss.Color("#ebbcba"), // rose
		OtherMsg: lipgloss.Color("#c4a7e7"), // iris
	}
}

// Names returns the list of available theme names.
var Names = []string{"catppuccin", "rose-pine", "kanagawa"}

// configPath returns ~/.config/chattui/theme.
func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "chattui", "theme")
}

// Load reads the saved theme from disk and applies it. Falls back to env/default.
func Load(envFallback string) {
	data, err := os.ReadFile(configPath())
	if err == nil {
		name := strings.TrimSpace(string(data))
		if name != "" {
			SetTheme(name)
			return
		}
	}
	SetTheme(envFallback)
}

// Save persists the current theme name to disk.
func Save() error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(Current.Name), 0o644)
}

// KanagawaDragon returns the Kanagawa Dragon color palette.
func KanagawaDragon() Theme {
	return Theme{
		Name:     "kanagawa",
		Base:     lipgloss.Color("#181616"),
		Surface:  lipgloss.Color("#282727"),
		Overlay:  lipgloss.Color("#393836"),
		Text:     lipgloss.Color("#c5c9c5"),
		Subtle:   lipgloss.Color("#727169"),
		Accent:   lipgloss.Color("#7e9cd8"), // crystal blue
		Second:   lipgloss.Color("#7fb4ca"), // spring blue
		Gold:     lipgloss.Color("#e6c384"), // carp yellow
		Error:    lipgloss.Color("#c34043"), // samurai red
		Success:  lipgloss.Color("#98bb6c"), // spring green
		OwnMsg:   lipgloss.Color("#d27e99"), // sakura pink
		OtherMsg: lipgloss.Color("#7fb4ca"), // spring blue
	}
}
