package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Status holds the state shown in the bottom status line. Callers own
// the state and pass a snapshot. StatusBar keeps no state.
type Status struct {
	Room    int64
	Title   string
	Live    bool
	Conn    ConnState
	Watched string // preformatted value, for example "1.2万"
	Fans    string
	Pop     int64 // room popularity from heartbeats. 0 hides the segment
	Area    string
}

// ConnState is the danmaku connection state shown in the bar.
type ConnState int

// Connection states.
const (
	ConnConnecting ConnState = iota
	ConnOnline
	ConnRetrying
	ConnOffline
)

// StyleFuncs colors the powerline segments. The theme provides them.
type StyleFuncs struct {
	// Segment renders a bold chunk with 1-cell side padding on fg and bg.
	Segment func(fg, bg, text string) string
	// Title renders the room title chunk on its dim background.
	Title func(text string) string
	// Meta renders dim metadata for viewers, fans, and hearts.
	Meta func(text string) string
}

// StatusBar renders the tmux-style powerline row. It gives the title
// whatever width is left after fixed chrome.
type StatusBar struct {
	icons  Icons
	styles StyleFuncs
}

// NewStatusBar builds a status bar renderer.
func NewStatusBar(icons Icons, styles StyleFuncs) *StatusBar {
	return &StatusBar{icons: icons, styles: styles}
}

// Render builds the full status line for the given width in cells.
func (b *StatusBar) Render(st Status, width int) string {
	if width < 20 {
		width = 20
	}

	liveSeg := PowerSegment("252", "240", " "+b.icons.Offline+" OFFLINE")
	if st.Live {
		liveSeg = PowerSegment("252", "196", " "+b.icons.Live+" LIVE")
	}
	var areaSeg string
	if area := strings.TrimSpace(st.Area); area != "" {
		areaSeg = PowerSegment("250", "238", area)
	}
	connText, connColor := b.conn(st.Conn)

	var mid strings.Builder
	if st.Watched != "" {
		mid.WriteString(" " + b.icons.Watched + st.Watched)
	}
	if st.Fans != "" {
		mid.WriteString(" " + b.icons.Fans + st.Fans)
	}
	if st.Pop > 0 {
		mid.WriteString(" " + b.icons.Hearts + FormatCount(st.Pop))
	}
	var metaSeg string
	if mid.Len() > 0 {
		metaSeg = b.styles.Meta(mid.String())
	}

	head := PowerSegment("231", "236", " "+strconv.FormatInt(st.Room, 10)) + liveSeg + areaSeg

	tail := PowerSegment("252", connColor, connText) +
		PowerSegment("231", "236", time.Now().Format("15:04:05")+" ")
	budget := width - lipgloss.Width(head) - lipgloss.Width(metaSeg) - lipgloss.Width(tail)
	// On narrow widths, drop metadata, then area, before shrinking title.
	if budget < 10 && metaSeg != "" {
		budget += lipgloss.Width(metaSeg)
		metaSeg = ""
	}
	if budget < 10 && areaSeg != "" {
		budget += lipgloss.Width(areaSeg)
		areaSeg = ""
		head = PowerSegment("231", "236", " "+strconv.FormatInt(st.Room, 10)) + liveSeg
	}
	title := b.styles.Title(" " + fitTitle(st.Title, budget-3))

	return head + title + metaSeg + tail
}

func (b *StatusBar) conn(c ConnState) (text, color string) {
	switch c {
	case ConnOnline:
		return " " + joinIcon(b.icons.LinkOK, "ONLINE"), "29"
	case ConnRetrying:
		return " " + joinIcon(b.icons.LinkRetry, "RETRYING"), "136"
	case ConnConnecting:
		return " " + joinIcon(b.icons.LinkWait, "CONNECTING"), "136"
	default:
		return " " + joinIcon(b.icons.LinkOff, "OFFLINE"), "240"
	}
}

func joinIcon(icon, text string) string {
	if icon == "" {
		return text
	}
	return icon + " " + text
}

// PowerSegment renders the default tmux-style powerline chunk. It shows
// bold text on a fg and bg pair with one cell of padding on each side.
func PowerSegment(fg, bg, text string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(fg)).
		Background(lipgloss.Color(bg)).
		Bold(true).
		Padding(0, 1).
		Render(text)
}

// fitTitle cuts the title rune-wise to max visible cells. It ends the cut with an ellipsis.
func fitTitle(title string, max int) string {
	if max < 2 {
		return ""
	}
	if lipgloss.Width(title) <= max {
		return title
	}
	for title != "" && lipgloss.Width(title+"…") > max {
		r := []rune(title)
		title = string(r[:len(r)-1])
	}
	return title + "…"
}
