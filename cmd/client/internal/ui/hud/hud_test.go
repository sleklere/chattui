package hud

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestFrameRenderFillsTerminal(t *testing.T) {
	frame := Frame{
		Width:     80,
		Height:    24,
		ActiveTab: TabRooms,
		Badges:    map[string]int64{TabInbox: 12},
		Keys:      []Key{{Key: "↵", Label: "join"}},
	}

	out := frame.Render("body")
	lines := strings.Split(out, "\n")

	if len(lines) != 24 {
		t.Fatalf("frame height = %d lines, want 24", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("line %d is %d columns wide, want at most 80", i, w)
		}
	}
}

func TestBodyHeightNeverGoesBelowOne(t *testing.T) {
	if got := BodyHeight(2); got != 1 {
		t.Errorf("BodyHeight(2) = %d, want 1", got)
	}
	if got := BodyHeight(24); got != 24-Height {
		t.Errorf("BodyHeight(24) = %d, want %d", got, 24-Height)
	}
}

func TestOverlayKeepsBodyDimensions(t *testing.T) {
	body := strings.Repeat("x", 40)
	out := Overlay(strings.Join([]string{body, body, body, body, body, body}, "\n"), Modal("Title", "content"), 40, 6)

	lines := strings.Split(out, "\n")
	if len(lines) != 6 {
		t.Fatalf("overlay height = %d lines, want 6", len(lines))
	}
}
