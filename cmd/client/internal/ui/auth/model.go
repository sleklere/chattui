// Package auth provides the authentication UI model for the chat client.
package auth

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/ui/components"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
	"github.com/sleklere/chattui/pkg/dto"
)

// wordmark is the chattui logo shown above the login card.
var wordmark = []string{
	"┌─┐┬ ┬┌─┐┌┬┐┌┬┐┬ ┬┬",
	"│  ├─┤├─┤ │  │ │ ││",
	"└─┘┴ ┴┴ ┴ ┴  ┴ └─┘┴",
}

// fieldWidth is the number of columns a login field renders into: the 40-column
// card body less the two-space indent, minus the trailing cell Bubbles keeps for
// the cursor. Bubbles v2 needs an explicit width or it clips the placeholder to
// its first character.
const fieldWidth = 37

type mode int

const (
	modeLogin mode = iota
	modeRegister
)

// SuccessMsg signals a successful authentication with token and user info.
type SuccessMsg struct {
	Token    string
	UserID   int64
	Username string
}

// ErrorMsg signals a failed authentication attempt.
type ErrorMsg struct {
	Err error
}

// Model is the Bubble Tea model for the authentication screen.
type Model struct {
	apiClient     *api.Client
	usernameInput textinput.Model
	passwordInput textinput.Model
	spinner       spinner.Model
	mode          mode
	focusIndex    int
	err           string
	loading       bool
	width         int
	height        int
}

// New creates a new auth Model with the given API client.
func New(apiClient *api.Client) Model {
	username := textinput.New()
	username.Prompt = ""
	username.Placeholder = "username"
	username.Focus()
	username.CharLimit = 32
	username.SetWidth(fieldWidth)
	username.SetStyles(components.InputStyles())

	password := textinput.New()
	password.Prompt = ""
	password.Placeholder = "password"
	password.EchoMode = textinput.EchoPassword
	password.CharLimit = 64
	password.SetWidth(fieldWidth)
	password.SetStyles(components.InputStyles())

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.Current.Accent)

	return Model{
		apiClient:     apiClient,
		usernameInput: username,
		passwordInput: password,
		spinner:       s,
		mode:          modeLogin,
	}
}

// Init initializes the auth model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

// Update handles messages for the auth model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.err = ""
		switch msg.String() {
		case "tab", "shift+tab":
			return m.cycleFocus(), nil
		case "ctrl+t":
			m.toggleMode()
			return m, nil
		case "enter":
			if m.loading {
				return m, nil
			}
			cmd := m.submit()
			m.loading = true
			return m, tea.Batch(cmd, m.spinner.Tick)
		}

	case SuccessMsg:
		m.loading = false

	case ErrorMsg:
		m.loading = false
		m.err = msg.Err.Error()

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m.updateInputs(msg)
}

// View renders the auth model.
func (m Model) View() string {
	t := theme.Current

	logo := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(strings.Join(wordmark, "\n"))
	tagline := lipgloss.NewStyle().Foreground(t.Subtle).Render("chat that lives in your terminal")

	title := "Sign in"
	if m.mode == modeRegister {
		title = "Create account"
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(t.Gold).Bold(true).Render(title))
	b.WriteString("\n\n")
	b.WriteString(m.field("Username", m.usernameInput, m.focusIndex == 0))
	b.WriteString("\n\n")
	b.WriteString(m.field("Password", m.passwordInput, m.focusIndex == 1))
	b.WriteString("\n\n")
	b.WriteString(m.statusLine())

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Overlay).
		Padding(1, 3).
		Width(48). // border included in v2: 46 of card plus its two edges
		Render(b.String())

	toggle := "no account yet?  ctrl+t to register"
	if m.mode == modeRegister {
		toggle = "already registered?  ctrl+t to sign in"
	}
	help := lipgloss.NewStyle().Foreground(t.Subtle).Render(toggle) + "\n" +
		lipgloss.NewStyle().Foreground(t.Overlay).Render("tab switch field   enter submit   ctrl+c quit")

	screen := lipgloss.JoinVertical(lipgloss.Center, logo, "", tagline, "", card, "", help)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, screen)
}

// field renders a labelled input with an underline that lights up when focused.
func (m Model) field(label string, input textinput.Model, focused bool) string {
	t := theme.Current

	labelColor := t.Subtle
	borderColor := t.Overlay
	marker := "  "
	if focused {
		labelColor = t.Accent
		borderColor = t.Accent
		marker = lipgloss.NewStyle().Foreground(t.Accent).Render("▸ ")
	}

	head := marker + lipgloss.NewStyle().Foreground(labelColor).Bold(true).Render(label)
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(borderColor).
		Width(40).
		Render("  " + input.View())

	return head + "\n" + box
}

// statusLine shows the spinner while authenticating, or the last error.
func (m Model) statusLine() string {
	t := theme.Current
	switch {
	case m.loading:
		return m.spinner.View() + lipgloss.NewStyle().Foreground(t.Gold).Render(" authenticating")
	case m.err != "":
		return lipgloss.NewStyle().Foreground(t.Error).Render("✗ " + m.err)
	default:
		return " "
	}
}

func (m *Model) toggleMode() {
	if m.mode == modeLogin {
		m.mode = modeRegister
	} else {
		m.mode = modeLogin
	}
	m.err = ""
}

func (m Model) cycleFocus() Model {
	if m.focusIndex == 0 {
		m.focusIndex = 1
		m.usernameInput.Blur()
		m.passwordInput.Focus()
	} else {
		m.focusIndex = 0
		m.passwordInput.Blur()
		m.usernameInput.Focus()
	}
	return m
}

func (m Model) submit() tea.Cmd {
	username := strings.TrimSpace(m.usernameInput.Value())
	password := m.passwordInput.Value()

	if username == "" || password == "" {
		return func() tea.Msg {
			return ErrorMsg{Err: fmt.Errorf("username and password are required")}
		}
	}

	req := api.AuthRequest{Username: username, Password: password}

	return func() tea.Msg {
		var res dto.Auth
		var err error

		if m.mode == modeLogin {
			res, err = m.apiClient.Login(req)
		} else {
			res, err = m.apiClient.Register(req)
		}

		if err != nil {
			return ErrorMsg{Err: err}
		}

		return SuccessMsg{
			Token:    res.Token,
			UserID:   res.User.ID,
			Username: res.User.Username,
		}
	}
}

func (m Model) updateInputs(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.usernameInput, cmd = m.usernameInput.Update(msg)
	cmds = append(cmds, cmd)

	m.passwordInput, cmd = m.passwordInput.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}
