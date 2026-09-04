// Package app connects the bilibili client to the terminal UI. It runs the watch loop.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Arcadi4/bililive-cli/bili"
	"github.com/Arcadi4/bililive-cli/config"
	"github.com/Arcadi4/bililive-cli/ui"
)

// WatchOptions holds settings for the watch loop.
type WatchOptions struct {
	// RoomSpec is a room id, a short id, or a live.bilibili.com URL.
	RoomSpec string
	Session  *config.Session
	Prefs    config.Preferences
	// Quiet hides enter and like messages.
	Quiet bool
	// Unicode uses plain unicode symbols in place of nerd-font glyphs.
	// Use it on terminals without a nerd font.
	Unicode bool
	// Logger receives protocol diagnostics. Nil disables logging.
	Logger *slog.Logger
	// Plain forces non-interactive output for pipes.
	Plain bool
	// ShowAll shows unknown protocol messages as raw lines.
	ShowAll bool
}

// Watch runs the viewer until ctx ends. Danmaku loop errors
// return here only when they stop the first connection.
func Watch(ctx context.Context, opts WatchOptions) error {
	roomID, err := ParseRoom(opts.RoomSpec)
	if err != nil {
		return err
	}
	if opts.Quiet {
		opts.Prefs.ShowEntrants = false
		opts.Prefs.ShowLikes = false
	}

	client := bili.NewClient(opts.Session)
	ri, err := client.ResolveRoom(ctx, roomID)
	if err != nil {
		return fmt.Errorf("resolve room: %w", err)
	}
	info, err := client.RoomInfo(ctx, ri.RoomID)
	if err != nil {
		return fmt.Errorf("room info: %w", err)
	}

	screen := ui.NewScreen(os.Stdout)
	renderer := ui.NewRenderer(ui.NewTheme(), opts.Prefs, opts.Unicode)
	renderer.ShowAll = opts.ShowAll
	renderer.RoomID = ri.RoomID
	if opts.Session != nil && opts.Session.HasSession() {
		renderer.SelfID = opts.Session.DedeUserID
	}
	statusBar := ui.NewStatusBar(ui.StatusIcons(opts.Unicode), ui.StyleFuncs{
		Title: func(text string) string { return ui.PowerSegment("223", "236", text) },
		Meta:  func(text string) string { return ui.PowerSegment("244", "234", text) },
	})

	area := strings.TrimSpace(info.ParentName + "·" + info.AreaName)
	if area == "·" {
		area = ""
	}

	var (
		mu      sync.Mutex
		live    = info.LiveStatus == 1
		conn    = ui.ConnConnecting
		watched string
		fans    string
		pop     = info.Online
	)

	statusFn := func(width int) string {
		mu.Lock()
		st := ui.Status{
			Room:    ri.RoomID,
			Title:   info.Title,
			Live:    live,
			Conn:    conn,
			Watched: watched,
			Fans:    fans,
			Pop:     pop,
			Area:    area,
		}
		mu.Unlock()
		return statusBar.Render(st, width)
	}

	inputBar := ui.NewInputBar()
	inputBar.SetPlaceholderText(placeholderFor(opts.Session))
	inputFn := func(width int) (string, int) { return inputBar.Render(width) }

	// Raw mode blocks SIGINT. runCtx ends on Ctrl+C with an empty
	// input bar. It also ends on Ctrl+D or when the parent context ends.
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	if !opts.Plain {
		if err := screen.Start(statusFn, inputFn); err != nil {
			return fmt.Errorf("init screen: %w", err)
		}
		// Keep the status line clock running between events.
		clockStop := make(chan struct{})
		defer close(clockStop)
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					screen.Repaint()
				case <-clockStop:
					return
				}
			}
		}()
		inputBar.Submit = func(text string) {
			go func() {
				if err := client.SendComment(runCtx, ri.RoomID, text, 16777215); err != nil {
					screen.Write(renderer.RenderSendError(err))
					return
				}
				screen.Write(renderer.RenderSelfComment(text))
			}()
		}
		inputBar.Quit = runCancel
		go func() {
			reader := newInputReader(os.Stdin)
			for {
				ev, ok := reader.next()
				if !ok {
					runCancel()
					return
				}
				if inputBar.Feed(ev) {
					continue
				}
				screen.Repaint()
			}
		}()
	} else {
		screen.Write(fmt.Sprintf("watching room %d: \"%s\"", ri.RoomID, info.Title))
	}

	dc := &bili.DanmakuClient{
		RoomID:  ri.RoomID,
		UID:     sessionUID(opts.Session),
		Session: opts.Session,
		Logger:  opts.Logger,
		OnEvent: func(ev bili.Event) {
			// Update the bar state from metadata events.
			switch ev.Cmd {
			case "WATCHED_CHANGE":
				var m struct {
					Data struct {
						Num       int64  `json:"num"`
						TextSmall string `json:"text_small"`
					} `json:"data"`
				}
				if jsonOk(ev.Raw, &m) {
					mu.Lock()
					watched = m.Data.TextSmall
					mu.Unlock()
				}
			case "ROOM_REAL_TIME_MESSAGE_UPDATE":
				var m struct {
					Data struct {
						Fans int64 `json:"fans"`
					} `json:"data"`
				}
				if jsonOk(ev.Raw, &m) {
					mu.Lock()
					fans = ui.FormatCount(m.Data.Fans)
					mu.Unlock()
				}
			case "LIVE":
				mu.Lock()
				live = true
				mu.Unlock()
			case "PREPARING":
				mu.Lock()
				live = false
				mu.Unlock()
			}
			line, ok := renderer.Render(ev)
			if !ok || line == "" {
				return
			}
			screen.Write(line)
		},
		OnStatus: func(connected bool, err error) {
			mu.Lock()
			switch {
			case connected:
				conn = ui.ConnOnline
			case err != nil:
				conn = ui.ConnRetrying
			default:
				conn = ui.ConnOffline
			}
			mu.Unlock()
			if err != nil {
				if opts.Logger != nil {
					opts.Logger.Info("connection lost", "error", err)
				}
				screen.Write(renderer.RenderReconnect(err))
			}
		},
		OnPopularity: func(p int64) {
			mu.Lock()
			pop = p
			mu.Unlock()
		},
	}

	err = dc.Run(runCtx)
	screen.Stop()
	return err
}

func placeholderFor(s *config.Session) string {
	if s != nil && s.LoggedIn() {
		return "输入弹幕，回车发送 · Ctrl+C 清空/退出"
	}
	if s != nil && s.HasSession() {
		return "已登录但缺少 bili_jct，无法发弹幕 · Ctrl+C 清空/退出"
	}
	return "游客模式（bililive login 后可发弹幕）· Ctrl+C 清空/退出"
}

// ParseRoom parses a raw id or a live.bilibili.com URL.
func ParseRoom(spec string) (int64, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, fmt.Errorf("room required")
	}
	if strings.Contains(spec, "live.bilibili.com") {
		rest := spec[strings.Index(spec, "live.bilibili.com/")+len("live.bilibili.com/"):]
		rest = strings.TrimPrefix(rest, "h5/")
		for _, sep := range []string{"?", "/", "#"} {
			if j := strings.Index(rest, sep); j >= 0 {
				rest = rest[:j]
			}
		}
		spec = rest
	}
	id, err := strconv.ParseInt(spec, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid room %q", spec)
	}
	return id, nil
}

func sessionUID(s *config.Session) int64 {
	if s == nil {
		return 0
	}
	return s.DedeUserID
}

func jsonOk(raw []byte, out any) bool {
	return raw != nil && json.Unmarshal(raw, out) == nil
}
