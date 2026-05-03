// Package auth provides the authentication UI model for the chat client.
package auth

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
)

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
	username.Placeholder = "username"
	username.Focus()
	username.CharLimit = 32

	password := textinput.New()
	password.Placeholder = "password"
	password.EchoMode = textinput.EchoPassword
	password.CharLimit = 64

	return Model{
		apiClient:     apiClient,
		usernameInput: username,
		passwordInput: password,
		mode:          modeLogin,
	}
}

// Init initializes the auth model.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the auth model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			return m, m.submit()
		}

	case SuccessMsg:
		m.loading = false

	case ErrorMsg:
		m.loading = false
		m.err = msg.Err.Error()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m.updateInputs(msg)
}

// View renders the auth model.
func (m Model) View() string {
	t := theme.Current

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent)

	formStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Surface).
		Padding(1, 3).
		Width(50)

	labelStyle := lipgloss.NewStyle().
		Foreground(t.Subtle).
		Bold(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(t.Error)

	helpStyle := lipgloss.NewStyle().
		Foreground(t.Subtle).
		Italic(true)

	brandStyle := lipgloss.NewStyle().
		Foreground(t.Gold).
		Bold(true)

	var title string
	if m.mode == modeLogin {
		title = "Login"
	} else {
		title = "Register"
	}

	var b strings.Builder

	b.WriteString(brandStyle.Render("chattui"))
	b.WriteString("\n\n")
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("Username"))
	b.WriteString("\n")
	b.WriteString(m.usernameInput.View())
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("Password"))
	b.WriteString("\n")
	b.WriteString(m.passwordInput.View())
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString(lipgloss.NewStyle().Foreground(t.Gold).Render("Authenticating..."))
	} else if m.err != "" {
		b.WriteString(errorStyle.Render(m.err))
	}

	b.WriteString("\n\n")

	var modeToggle string
	if m.mode == modeLogin {
		modeToggle = "Don't have an account? ctrl+t to register"
	} else {
		modeToggle = "Already have an account? ctrl+t to login"
	}
	b.WriteString(helpStyle.Render(fmt.Sprintf("%s\ntab: switch fields  enter: submit", modeToggle)))

	form := formStyle.Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, form)
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

	m.loading = true
	req := api.AuthRequest{Username: username, Password: password}

	return func() tea.Msg {
		var res api.AuthResponse
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
