// Package ui renders the watch experience. It shows a native-scrolling
// log of room events. It pins a tmux-style two-line bar at the bottom.
package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme holds the palette for event rendering. Create one instance and share it.
type Theme struct {
	// Bar styles the bottom metadata line with room title, viewers, and status.
	Bar lipgloss.Style
	// BarDim styles dimmed metadata segments inside the bar.
	BarDim lipgloss.Style
	Time   lipgloss.Style
	Name   lipgloss.Style
	// Medal styles fan medals as a pill badge, for example "粉丝团 Lv12".
	Medal lipgloss.Style
	// GuardBadge styles guard subscriptions as a distinct pill. It covers 舰长, 提督, and 总督.
	GuardBadge lipgloss.Style
	// Danmaku styles comment text.
	Danmaku lipgloss.Style
	Gift    lipgloss.Style
	// Guard styles guard purchases for 舰长, 提督, and 总督.
	Guard lipgloss.Style
	// SuperChat styles SC lines.
	SuperChat lipgloss.Style
	// Info styles lifecycle and status lines such as live start and stream cut.
	Info lipgloss.Style
	// Enter styles viewer enter lines and follow lines.
	Enter lipgloss.Style
	Like  lipgloss.Style
	// Notice styles system-wide notices for NOTICE_MSG.
	Notice lipgloss.Style
	// Self styles lines about the logged-in user, such as own comments.
	Self  lipgloss.Style
	Error lipgloss.Style
}

// NewTheme builds the default theme. It uses ANSI256 colors to support many terminals.
func NewTheme() *Theme {
	t := &Theme{
		Bar:        lipgloss.NewStyle().Bold(true),
		BarDim:     lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		Time:       lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		Name:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
		Medal:      lipgloss.NewStyle().Background(lipgloss.Color("24")).Foreground(lipgloss.Color("231")).Bold(true).Padding(0, 1),
		GuardBadge: lipgloss.NewStyle().Background(lipgloss.Color("130")).Foreground(lipgloss.Color("231")).Bold(true).Padding(0, 1),
		Danmaku: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),
		Gift:      lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		Guard:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("135")),
		SuperChat: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("204")),
		Info:      lipgloss.NewStyle().Foreground(lipgloss.Color("111")),
		Enter:     lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		Like:      lipgloss.NewStyle().Foreground(lipgloss.Color("192")),
		Notice:    lipgloss.NewStyle().Foreground(lipgloss.Color("179")),
		Self:      lipgloss.NewStyle().Foreground(lipgloss.Color("117")),
		Error:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("204")),
	}
	return t
}
