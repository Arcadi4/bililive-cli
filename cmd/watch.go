package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/Arcadi4/bililive-cli/app"
	"github.com/Arcadi4/bililive-cli/bili"
	"github.com/Arcadi4/bililive-cli/config"
	"github.com/spf13/cobra"
)

var (
	flagAll     bool
	flagQuiet   bool
	flagPlain   bool
	flagDebug   bool
	flagUnicode bool
)

var watchCmd = &cobra.Command{
	Use:   "watch [room]",
	Short: "Monitor a live room's interactions in the terminal",
	Long: `Monitor a bilibili live room and log audience interactions:
	
	- danmaku (comments)
	- gifts
	- subscription purchases
	- Super Chats
	- enters
	- likes
	- follower events
	- other room status changes.

With no ROOM, watch your own room (needs login). ROOM is an id, short
id, or live.bilibili.com URL.`,
	Example: `# Your own room, after login
bililive watch

# Watch a room by id, short id, or URL
bililive watch 6
bililive watch 1878516995
bililive watch https://live.bilibili.com/1878516995

# Show only gifts and comments, hide noise
bililive watch 1878516995 --quiet

# Display all data from bilibili's WebSocket
bililive watch 1878516995 --all

# Plain mode when piped
bililive watch 1878516995 | tee room.log`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		session, err := config.LoadSession()
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning: load session:", err)
		}
		prefs, err := config.LoadPreferences()
		if err != nil {
			prefs = config.DefaultPreferences()
		}
		var logger *slog.Logger
		if flagDebug {
			logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		roomSpec := ""
		if len(args) > 0 {
			roomSpec = args[0]
		} else if session == nil || !session.HasSession() {
			return fmt.Errorf("no room specified and no stored session: run `bililive login` first, or pass a room id")
		} else {
			roomID, err := bili.NewClient(session).OwnRoomID(ctx)
			if err != nil {
				return err
			}
			roomSpec = strconv.FormatInt(roomID, 10)
		}
		watchErr := app.Watch(ctx, app.WatchOptions{
			RoomSpec: roomSpec,
			Session:  session,
			Prefs:    prefs,
			Quiet:    flagQuiet,
			ShowAll:  flagAll,
			Unicode:  flagUnicode,
			Plain:    flagPlain,
			Logger:   logger,
		})
		if errors.Is(watchErr, context.Canceled) {
			return nil // Ctrl+C or EOF ends the watch cleanly.
		}
		return watchErr
	},
}

func init() {
	watchCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false, "hide enter/like noise, keep gifts/comments/system")
	watchCmd.Flags().BoolVar(&flagAll, "all", false, "show raw protocol messages and metadata lines (cross-room notices, like counts, rankings, viewers)")
	watchCmd.Flags().BoolVar(&flagPlain, "plain", false, "disable the bottom bar and key input (pipe mode)")
	watchCmd.Flags().BoolVar(&flagDebug, "debug", false, "log protocol diagnostics to stderr")
	watchCmd.Flags().BoolVar(&flagUnicode, "unicode", false, "use plain unicode symbols instead of nerd font glyphs")
}
