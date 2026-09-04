package ui

// Icons holds the symbols for log lines and the status bar. The
// nerd font set is the default. The unicode set is the --unicode
// fallback for terminals without a nerd font.
type Icons struct {
	Live      string // room is broadcasting
	Offline   string // room is not broadcasting
	LinkOK    string // danmaku connection established
	LinkRetry string // danmaku connection retrying
	LinkWait  string // danmaku connection being established
	LinkOff   string // danmaku connection down
	Watched   string // viewers who have watched
	Fans      string // fan club size
	Hearts    string // likes / popularity
	Play      string // live started
	Stop      string // stream ended
	Warn      string // stream cut off
	Ban       string // user muted
	Bolt      string // online rank
}

// UnicodeIcons returns the fallback symbol set in plain unicode for --unicode.
func UnicodeIcons() Icons {
	return Icons{
		Live:    "●",
		Offline: "○",
		Watched: "👁",
		Fans:    "✦",
		Hearts:  "♥",
		Play:    "▶",
		Stop:    "■",
		Warn:    "⚠",
		Ban:     "⛔",
		Bolt:    "⚡",
	}
}

// NerdIcons returns the default nerd font symbol set.
func NerdIcons() Icons {
	return Icons{
		Live:      "\uf111", // nf-fa-circle
		Offline:   "\uf1db", // nf-fa-circle_o
		LinkOK:    "\uf0c1", // nf-fa-link
		LinkRetry: "\uf021", // nf-fa-refresh
		LinkWait:  "\uf110", // nf-fa-spinner

		LinkOff: "\uf127", // nf-fa-chain_broken
		Watched: "\uf06e", // nf-fa-eye
		Fans:    "\uf005", // nf-fa-star
		Hearts:  "\uf004", // nf-fa-heart
		Play:    "\uf04b", // nf-fa-play
		Stop:    "\uf04d", // nf-fa-stop
		Warn:    "\uf071", // nf-fa-warning
		Ban:     "\uf05e", // nf-fa-ban
		Bolt:    "\uf0e7", // nf-fa-bolt
	}
}

// StatusIcons returns the icon set for the status bar. unicode=true
// selects the plain unicode fallback.
func StatusIcons(unicode bool) Icons {
	if unicode {
		return UnicodeIcons()
	}
	return NerdIcons()
}
