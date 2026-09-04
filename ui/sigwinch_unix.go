//go:build !windows

package ui

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyWinch relays terminal resize signals to c. SIGWINCH does not
// exist on Windows, so the Windows implementation is a no-op and the
// bar relayouts on the next repaint instead.
func notifyWinch(c chan os.Signal) {
	signal.Notify(c, syscall.SIGWINCH)
}
