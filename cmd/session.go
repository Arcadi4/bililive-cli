package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Arcadi4/bililive-cli/app"
	"github.com/Arcadi4/bililive-cli/bili"
	"github.com/Arcadi4/bililive-cli/config"
	"github.com/spf13/cobra"
)

var loginFlagCookie string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to bilibili with a QR code",
	Long: `Log in by scanning a QR code with the bilibili app and confirming on your phone.

Pass an existing browser Cookie header with --cookie only when QR login is unavailable.
The import requires SESSDATA; bili_jct enables comment sending.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if strings.TrimSpace(loginFlagCookie) != "" {
			s := &config.Session{}
			parseCookieHeader(s, loginFlagCookie)
			if !s.HasSession() {
				return fmt.Errorf("SESSDATA is required")
			}
			return completeLogin(ctx, s, cmd.OutOrStdout())
		}

		client := bili.NewClient(nil)
		buvid3, err := client.DeviceID(ctx)
		if err != nil {
			return err
		}
		login, err := client.NewQRLogin(ctx)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "请使用哔哩哔哩客户端扫描二维码登录：")
		if err := renderLoginQRCode(out, login.URL); err != nil {
			return err
		}
		fmt.Fprintln(out, "扫描后请在手机上确认。")

		pollCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		state := bili.QRLoginWaiting
		for {
			status, err := client.PollQRLogin(pollCtx, login.Key)
			if err != nil {
				if pollCtx.Err() != nil {
					return fmt.Errorf("QR login cancelled or timed out: %w", pollCtx.Err())
				}
				return err
			}
			switch status.State {
			case bili.QRLoginWaiting:
			case bili.QRLoginScanned:
				if state != status.State {
					fmt.Fprintln(out, "已扫码，请在手机上确认。")
				}
			case bili.QRLoginExpired:
				return fmt.Errorf("二维码已过期，请重新运行 `bili login`")
			case bili.QRLoginSuccess:
				status.Session.BUVID3 = buvid3
				return completeLogin(pollCtx, status.Session, out)
			}
			state = status.State
			select {
			case <-pollCtx.Done():
				return fmt.Errorf("QR login cancelled or timed out: %w", pollCtx.Err())
			case <-ticker.C:
			}
		}
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the stored session",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.ClearSession(); err != nil {
			return err
		}
		fmt.Println("session cleared")
		return nil
	},
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Display the currently logged-in user",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := config.LoadSession()
		if err != nil {
			return err
		}
		if s == nil || !s.HasSession() {
			fmt.Println("not logged in, use `bililive login`")
			return nil
		}
		client := bili.NewClient(s)
		nav, err := client.Nav(context.Background())
		if err != nil {
			return err
		}
		if !nav.IsLogin {
			fmt.Println("stored cookies are present but bilibili says the session is NOT logged in (expired?)")
			return nil
		}
		if s.LoggedIn() {
			fmt.Printf("%s (mid %d)\n logged in, commenting enabled\n", nav.Uname, nav.MID)
		} else {
			fmt.Printf("%s (mid %d)\n logged in; bili_jct missing, commenting disabled (re-run `bililive login` with bili_jct)\n", nav.Uname, nav.MID)
		}
		return nil
	},
}

var (
	sendFlagColor int64
	sendFlagJSON  bool
)

var sendCmd = &cobra.Command{
	Use:   "send <room> <message>",
	Short: "Send one comment to a room and exit",
	Example: `bililive send 1878516995 "hello from the terminal"
  bililive send 23058 hi --color 16738671`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		roomID, err := app.ParseRoom(args[0])
		if err != nil {
			return err
		}
		s, err := config.LoadSession()
		if err != nil {
			return err
		}
		if s == nil || !s.HasSession() {
			return fmt.Errorf("not logged in: run `bililive login` first")
		}
		if !s.LoggedIn() {
			return fmt.Errorf("bili_jct (CSRF cookie) missing: re-run `bililive login` including bili_jct to enable sending")
		}
		client := bili.NewClient(s)
		ri, err := client.ResolveRoom(context.Background(), roomID)
		if err != nil {
			return fmt.Errorf("resolve room: %w", err)
		}
		if err := client.SendComment(context.Background(), ri.RoomID, args[1], sendFlagColor); err != nil {
			return err
		}
		if sendFlagJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetEscapeHTML(false)
			return enc.Encode(map[string]any{"ok": true, "room": ri.RoomID, "msg": args[1]})
		}
		fmt.Printf("sent to room %d: %s\n", ri.RoomID, args[1])
		return nil
	},
}

func init() {
	sendCmd.Flags().Int64Var(&sendFlagColor, "color", 0xFFFFFF, "danmaku color as decimal RGB (e.g. 16738671)")
	sendCmd.Flags().BoolVar(&sendFlagJSON, "json", false, "output JSON result")
	loginCmd.Flags().StringVar(&loginFlagCookie, "cookie", "", "raw Cookie header value (SESSDATA=...; bili_jct=...; ...)")
}

func completeLogin(ctx context.Context, session *config.Session, out io.Writer) error {
	client := bili.NewClient(session)
	nav, err := client.Nav(ctx)
	if err != nil {
		return fmt.Errorf("verify session: %w", err)
	}
	if !nav.IsLogin || nav.MID == 0 {
		return fmt.Errorf("bilibili rejected the session; the previous session was kept")
	}
	session.DedeUserID = nav.MID
	if session.BUVID3 == "" {
		buvid3, err := client.DeviceID(ctx)
		if err != nil {
			return err
		}
		session.BUVID3 = buvid3
	}
	session.SavedAt = time.Now().Format(time.RFC3339)
	if err := config.SaveSession(session); err != nil {
		return err
	}
	if session.LoggedIn() {
		fmt.Fprintf(out, "已登录：%s (mid %d)，可以发送评论。\n", nav.Uname, nav.MID)
	} else {
		fmt.Fprintf(out, "已登录：%s (mid %d)，缺少 bili_jct，无法发送评论。\n", nav.Uname, nav.MID)
	}
	return nil
}

// parseCookieHeader fills a Session from a raw Cookie header value.
func parseCookieHeader(s *config.Session, header string) {
	for _, part := range strings.Split(header, ";") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "SESSDATA":
			s.SESSDATA = v
		case "bili_jct":
			s.BiliJCT = v
		case "DedeUserID":
			var mid int64
			if _, err := fmt.Sscanf(v, "%d", &mid); err == nil {
				s.DedeUserID = mid
			}
		case "buvid3":
			s.BUVID3 = v
		}
	}
}
