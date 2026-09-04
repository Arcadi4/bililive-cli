//go:build windows

package ui

import "os"

// notifyWinch is a no-op on Windows: the OS has no SIGWINCH, so the
// resize channel never fires and the bar relayouts on the next repaint.
func notifyWinch(c chan os.Signal) {
}
