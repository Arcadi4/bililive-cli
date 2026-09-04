package ui

import (
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	xinput "github.com/charmbracelet/x/input"
)

// placeholderStyle matches the default textinput idle hint.
var placeholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

// InputBar wraps a bubbles/textinput model for raw x/input events from
// stdin. Screen owns raw mode. Up and Down recall history, Submit fires
// on Enter. textinput keeps edit state and key handling; Render draws
// the line and reports the caret column, with no cursor cell of its own.
type InputBar struct {
	mu      sync.Mutex
	ti      textinput.Model
	history []string
	histIdx int // history cursor. len(history) means the live entry

	// offset and offsetRight bound the horizontal scroll window. See overflow.
	offset      int
	offsetRight int

	// Submit fires on Enter with the current value. The value is never empty.
	Submit func(msg string)
	// Quit fires on Ctrl+C with an empty input box. It also fires on Ctrl+D.
	Quit func()
}

// NewInputBar builds a focused input bar.
func NewInputBar() *InputBar {
	ti := textinput.New()
	ti.Placeholder = "输入弹幕回车发送 · Ctrl+C 清空/退出"
	ti.Prompt = "  "
	ti.CharLimit = 100
	ti.Focus()
	return &InputBar{ti: ti, histIdx: 0}
}

// Feed maps an x/input event to the textinput model. It returns true
// when the event acts as quit.
func (b *InputBar) Feed(ev xinput.Event) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch e := ev.(type) {
	case xinput.PasteEvent:
		km := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(e), Paste: true}
		b.ti, _ = b.ti.Update(km)
		return false
	case xinput.KeyPressEvent:
		return b.feedKey(e.Key())
	}
	return false
}

func (b *InputBar) feedKey(k xinput.Key) bool {
	quit := func() {
		if b.Quit != nil {
			b.Quit()
		}
	}
	switch {
	case k.Mod.Contains(xinput.ModCtrl) && k.Code == 'c':
		if b.ti.Value() != "" {
			b.ti.SetValue("")
			b.histIdx = len(b.history)
			return false
		}
		quit()
		return true
	case k.Mod.Contains(xinput.ModCtrl) && k.Code == 'd':
		quit()
		return true
	case k.Code == xinput.KeyEnter:
		v := b.ti.Value()
		if v != "" {
			b.history = append(b.history, v)
			b.histIdx = len(b.history)
			b.ti.SetValue("")
			if b.Submit != nil {
				b.Submit(v)
			}
		}
	case k.Code == xinput.KeyUp:
		b.historyPrev()
	case k.Code == xinput.KeyDown:
		b.historyNext()
	default:
		if km, ok := toKeyMsg(k); ok {
			b.ti, _ = b.ti.Update(km)
		}
	}
	return false
}

func (b *InputBar) historyPrev() {
	if len(b.history) == 0 {
		return
	}
	if b.histIdx == len(b.history) {
		// Leaving the live entry keeps it by moving back only.
		b.histIdx--
	} else if b.histIdx > 0 {
		b.histIdx--
	}
	if b.histIdx >= 0 && b.histIdx < len(b.history) {
		b.ti.SetValue(b.history[b.histIdx])
		b.ti.CursorEnd()
	}
}

func (b *InputBar) historyNext() {
	if len(b.history) == 0 || b.histIdx >= len(b.history) {
		return
	}
	b.histIdx++
	if b.histIdx >= len(b.history) {
		b.ti.SetValue("")
		b.histIdx = len(b.history)
	} else {
		b.ti.SetValue(b.history[b.histIdx])
		b.ti.CursorEnd()
	}
}

// Render draws the input line to fit the width, scrolling long comments
// sideways to stay on one row. It returns the 0-based caret column for
// the terminal cursor.
func (b *InputBar) Render(width int) (string, int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	promptW := lipgloss.Width(b.ti.Prompt)
	field := width - promptW
	if b.ti.Value() == "" && b.ti.Placeholder != "" {
		b.offset, b.offsetRight = 0, 0
		return b.ti.Prompt +
			placeholderStyle.Render(ansi.Truncate(b.ti.Placeholder, max(0, field), "…")), promptW
	}
	runes := []rune(b.ti.Value())
	pos := min(b.ti.Position(), len(runes))
	b.overflow(field, runes, pos)
	return b.ti.Prompt + string(runes[b.offset:b.offsetRight]),
		promptW + lipgloss.Width(string(runes[b.offset:pos]))
}

// overflow mirrors textinput handleOverflow. It scrolls the window from
// offset to offsetRight. This keeps the cursor (pos) visible in the field.
func (b *InputBar) overflow(field int, runes []rune, pos int) {
	if field <= 0 || lipgloss.Width(string(runes)) <= field {
		b.offset, b.offsetRight = 0, len(runes)
		return
	}
	// Correct the right edge after deletions shrank the value.
	b.offsetRight = min(b.offsetRight, len(runes))

	if pos < b.offset {
		b.offset = pos
		w, i := 0, 0
		for i < len(runes)-b.offset && w <= field {
			w += lipgloss.Width(string(runes[b.offset+i]))
			if w <= field+1 {
				i++
			}
		}
		b.offsetRight = b.offset + i
	} else if pos >= b.offsetRight {
		b.offsetRight = pos
		w := 0
		i := b.offsetRight - 1
		for i > 0 && w < field {
			w += lipgloss.Width(string(runes[i]))
			if w <= field {
				i--
			}
		}
		b.offset = b.offsetRight - (b.offsetRight - 1 - i)
	}
}

// SetPlaceholderText swaps the idle hint, for example after a login state change.
func (b *InputBar) SetPlaceholderText(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ti.Placeholder = text
}

// toKeyMsg maps an x/input Key to a bubbletea KeyMsg for textinput.
func toKeyMsg(k xinput.Key) (tea.KeyMsg, bool) {
	alt := k.Mod.Contains(xinput.ModAlt)
	if k.Text != "" && k.Code >= ' ' {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k.Text), Alt: alt}, true
	}
	if t, ok := specialKey[k.Code]; ok {
		return tea.KeyMsg{Type: t, Alt: alt}, true
	}
	// Ctrl+letter keys
	if k.Mod.Contains(xinput.ModCtrl) && k.Code >= 'a' && k.Code <= 'z' {
		t := ctrlKey(k.Code)
		if t != 0 {
			return tea.KeyMsg{Type: t}, true
		}
	}
	// Fallback: printable code point
	if k.Code >= ' ' {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{k.Code}, Alt: alt}, true
	}
	return tea.KeyMsg{}, false
}

var specialKey = map[rune]tea.KeyType{
	xinput.KeyEnter:     tea.KeyEnter,
	xinput.KeyTab:       tea.KeyTab,
	xinput.KeyBackspace: tea.KeyBackspace,
	xinput.KeyEscape:    tea.KeyEscape,
	xinput.KeySpace:     tea.KeySpace,
	xinput.KeyDelete:    tea.KeyDelete,
	xinput.KeyHome:      tea.KeyHome,
	xinput.KeyEnd:       tea.KeyEnd,
	xinput.KeyPgUp:      tea.KeyPgUp,
	xinput.KeyPgDown:    tea.KeyPgDown,
	xinput.KeyInsert:    tea.KeyInsert,
	xinput.KeyLeft:      tea.KeyLeft,
	xinput.KeyRight:     tea.KeyRight,
	xinput.KeyUp:        tea.KeyUp,
	xinput.KeyDown:      tea.KeyDown,
}

func ctrlKey(code rune) tea.KeyType {
	switch code {
	case 'a':
		return tea.KeyCtrlA
	case 'b':
		return tea.KeyCtrlB
	case 'd':
		return tea.KeyCtrlD
	case 'e':
		return tea.KeyCtrlE
	case 'f':
		return tea.KeyCtrlF
	case 'g':
		return tea.KeyCtrlG
	case 'h':
		return tea.KeyCtrlH
	case 'k':
		return tea.KeyCtrlK
	case 'l':
		return tea.KeyCtrlL
	case 'n':
		return tea.KeyCtrlN
	case 'p':
		return tea.KeyCtrlP
	case 'u':
		return tea.KeyCtrlU
	case 'w':
		return tea.KeyCtrlW
	case 'y':
		return tea.KeyCtrlY
	case '@':
		return tea.KeyCtrlAt
	case '[':
		return tea.KeyCtrlOpenBracket
	case '\\':
		return tea.KeyCtrlBackslash
	case ']':
		return tea.KeyCtrlCloseBracket
	case '^':
		return tea.KeyCtrlCaret
	case '_':
		return tea.KeyCtrlUnderscore
	}
	return 0
}
