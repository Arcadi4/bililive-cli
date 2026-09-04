package ui

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// Screen keeps a native-scrolling log with a pinned two-line bottom bar
// (status plus input) via a DECSTBM scroll region. Writes pass through
// as in any CLI.
//
// The terminal cursor doubles as the input caret. Each frame ends with
// the cursor on the input line. Each write starts with a line feed that
// scrolls the previous line up, then draws the new line just above the bar.
type Screen struct {
	w  io.Writer
	fd int

	mu         sync.Mutex
	width      int
	height     int
	prevHeight int // height at the previous layout. 0 before the first one.
	oldState   *term.State
	statusFn   func(width int) string
	inputFn    func(width int) (line string, cursorCol int)
	winch      chan os.Signal
	done       chan struct{}
	closed     bool
	lastLine   string // newest log line. It replays after a resize band-erase.
}

// NewScreen arms the bottom-bar layout on a TTY. Without a TTY, the
// screen passes output through without the bar, so piped output stays
// clean.
func NewScreen(w io.Writer) *Screen {
	s := &Screen{w: w}
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		s.fd = int(f.Fd())
	}
	return s
}

// Start enters raw mode, installs the scroll region, and draws the first
// bar. statusFn and inputFn run on each repaint with the current width.
// inputFn also reports the caret column for the native cursor.
func (s *Screen) Start(statusFn func(width int) string, inputFn func(width int) (line string, cursorCol int)) error {
	s.statusFn = statusFn
	s.inputFn = inputFn
	if s.fd == 0 {
		return nil // non-TTY passthrough
	}
	old, err := term.MakeRaw(s.fd)
	if err != nil {
		return fmt.Errorf("enter raw mode: %w", err)
	}
	s.oldState = old

	s.winch = make(chan os.Signal, 1)
	signal.Notify(s.winch, syscall.SIGWINCH)
	s.done = make(chan struct{})
	go func() {
		for {
			select {
			case <-s.winch:
				s.mu.Lock()
				closed := s.closed
				s.mu.Unlock()
				if closed {
					return
				}
				s.Relayout()
			case <-s.done:
				return
			}
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeLocked(syncOut(ansi.SetBracketedPasteMode + s.layoutLocked() + ansi.ShowCursor))
	return nil
}

// Stop tears down the bar. It resets the scroll region and restores the tty.
func (s *Screen) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.done != nil {
		close(s.done)
		s.done = nil
	}
	if s.winch != nil {
		signal.Stop(s.winch)
		s.winch = nil
	}
	if s.fd == 0 {
		return
	}
	s.writeLocked(
		ansi.CursorPosition(1, s.height-1) + ansi.EraseLine(2) +
			ansi.CursorPosition(1, s.height) + ansi.EraseLine(2) +
			ansi.ResetBracketedPasteMode +
			ansi.SetTopBottomMargins(1, s.height) +
			ansi.ShowCursor +
			ansi.CursorPosition(1, s.height),
	)
	if s.oldState != nil {
		_ = term.Restore(s.fd, s.oldState)
		s.oldState = nil
	}
}

// Write emits one log line into the scrolling region. A leading line
// feed scrolls the previous line up, and the new line lands directly
// above the bar with no blank row between.
func (s *Screen) Write(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fd == 0 {
		fmt.Fprintln(s.w, line)
		return
	}
	if s.closed {
		return
	}
	line = strings.TrimRight(line, "\n")
	if line == "" {
		return
	}
	s.lastLine = line
	s.writeLocked(syncOut(cup(s.height-2, 1) + "\n" + line + s.barLocked()))
}

// Relayout re-applies the scroll region, clears the stale bottom band, and
// repaints the bar. It runs on terminal resize.
func (s *Screen) Relayout() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fd == 0 || s.closed {
		return
	}
	s.writeLocked(syncOut(s.layoutLocked()))
}

// Repaint redraws only the pinned bar lines after a keystroke or a clock
// tick. It keeps the scroll region as is.
func (s *Screen) Repaint() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fd == 0 || s.closed {
		return
	}
	s.writeLocked(syncOut(s.barLocked()))
}

// layoutLocked rebuilds the bottom layout: scroll margins, a cleared
// bottom band, and a fresh bar. The clear removes stale bar fragments
// after a resize, but it also erases the newest log line, so the replay
// writes it back first. Callers hold mu.
func (s *Screen) layoutLocked() string {
	s.resize()
	from := s.height - 2
	if s.prevHeight > 0 {
		from = min(s.prevHeight, s.height) - 2
	}
	s.prevHeight = s.height
	replay := ""
	if s.lastLine != "" {
		replay = cup(s.height-2, 1) + ansi.Truncate(s.lastLine, s.width, "")
	}
	return ansi.SetTopBottomMargins(1, s.height-2) +
		cup(max(1, from), 1) +
		ansi.EraseDisplay(0) +
		replay +
		s.barLocked()
}

// barLocked redraws the two pinned rows over erased lines, then parks
// the native cursor at the text cursor column. Callers hold mu.
func (s *Screen) barLocked() string {
	line, col := s.inputLineLocked()
	return cup(s.height-1, 1) + ansi.EraseLine(2) + s.statusLineLocked() +
		cup(s.height, 1) + ansi.EraseLine(2) + line +
		cup(s.height, min(col+1, s.width))
}

func (s *Screen) statusLineLocked() string {
	if s.statusFn == nil {
		return ""
	}
	return ansi.Truncate(s.statusFn(s.width), s.width, "")
}

// inputLineLocked returns the rendered input line and the caret column
// (0-based) for the native cursor.
func (s *Screen) inputLineLocked() (string, int) {
	if s.inputFn == nil {
		return "", 0
	}
	line, col := s.inputFn(s.width)
	return ansi.Truncate(line, s.width, ""), col
}

// resize refreshes the cached terminal size. It falls back to 80x24.
func (s *Screen) resize() {
	w, h, err := term.GetSize(s.fd)
	if err != nil || w <= 0 || h <= 0 {
		w, h = 80, 24
	}
	s.width, s.height = w, h
}

// writeLocked writes to the underlying writer. Callers hold mu.
func (s *Screen) writeLocked(str string) {
	_, _ = io.WriteString(s.w, str)
}

// cup positions the cursor with clear (row, col) order.
// ansi.CursorPosition takes (col, row).
func cup(row, col int) string {
	return ansi.CursorPosition(col, row)
}

// syncOut wraps a frame in synchronized-output markers. The terminal
// then renders it atomically with no torn bar mid-redraw.
func syncOut(frame string) string {
	return ansi.SetModeSynchronizedOutput + frame + ansi.ResetModeSynchronizedOutput
}
