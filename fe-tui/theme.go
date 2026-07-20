package main

import "github.com/charmbracelet/lipgloss"

// Tokyo Night palette — mirrors the colors from the original fe.sh.
const (
	colBlue    = "#7aa2f7"
	colGreen   = "#9ece6a"
	colOrange  = "#e0af68"
	colRed     = "#f7768e"
	colComment = "#565f89"
	colFg      = "#c0caf5"
	colCyan    = "#7dcfff"
	colSelBg   = "#283457"
)

var (
	dirStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colBlue)).Bold(true)
	linkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colCyan))
	fileStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colFg))
	parentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colComment))
	cursorStyle  = lipgloss.NewStyle().Background(lipgloss.Color(colSelBg)).Bold(true)
	pointerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colBlue))

	headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colBlue)).Bold(true)
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colComment))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colGreen))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colOrange))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colRed))

	// Two-pane borders: accent for the active pane, dim for the inactive one.
	paneBorderActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colBlue))
	paneBorderDim = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colComment))

	metaStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colComment))
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colBlue))
	titleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colBlue)).Bold(true)
	helpKey     = lipgloss.NewStyle().Foreground(lipgloss.Color(colOrange)).Bold(true)
	helpDesc    = lipgloss.NewStyle().Foreground(lipgloss.Color(colFg))
)
