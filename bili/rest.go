// Package bili is a small client for the bilibili live APIs: REST calls
// (rooms, login, comments) plus the danmaku WebSocket protocol.
package bili

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Arcadi4/bililive-cli/config"
)

const (
	apiBase    = "https://api.live.bilibili.com"
	apiMain    = "https://api.bilibili.com"
	userAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36"
	httpExpiry = 10 * time.Second
)

// APIError is a non-zero code from a bilibili API response.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("bilibili api error %d: %s", e.Code, e.Message)
}

// Client sends REST calls to bilibili. It adds the stored session
// cookies when they exist.
type Client struct {
	HTTP    *http.Client
	Session *config.Session
	wbi     *wbiKeys
}

// NewClient returns a client that uses the given session. The session can be nil.
func NewClient(s *config.Session) *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: httpExpiry},
		Session: s,
	}
}

func (c *Client) get(ctx context.Context, endpoint string, q url.Values, out any) error {
	u := endpoint
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req, true)
	return c.do(req, out)
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		bytes.NewReader([]byte(form.Encode())))
	if err != nil {
		return err
	}
	c.setHeaders(req, true)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, out)
}

func (c *Client) setHeaders(req *http.Request, withCookie bool) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://live.bilibili.com/")
	req.Header.Set("Origin", "https://live.bilibili.com")
	if withCookie && c.Session != nil {
		if v := c.Session.CookieHeader(); v != "" {
			req.Header.Set("Cookie", v)
		}
	}
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", req.URL.Host, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d from %s: %s", resp.StatusCode, req.URL.Path,
			truncate(string(body), 200))
	}
	var envelope struct {
		APIError
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode %s: %w", req.URL.Path, err)
	}
	if envelope.Code != 0 {
		return &APIError{Code: envelope.Code, Message: envelope.Message}
	}
	if out != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("decode data of %s: %w", req.URL.Path, err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// RoomInit is the response from /room/v1/Room/room_init.
type RoomInit struct {
	RoomID      int64           `json:"room_id"`
	ShortID     int64           `json:"short_id"`
	UID         int64           `json:"uid"`
	LiveStatus  int             `json:"live_status"`
	LiveTime    json.RawMessage `json:"live_time"`
	Encrypted   bool            `json:"encrypted"`
	PwdVerified bool            `json:"pwd_verified"`
	IsHidden    bool            `json:"is_hidden"`
	IsLocked    bool            `json:"is_locked"`
}

// ResolveRoom maps a short room id or alias to the full room id.
func (c *Client) ResolveRoom(ctx context.Context, id int64) (*RoomInit, error) {
	q := url.Values{"id": {strconv.FormatInt(id, 10)}}
	var ri RoomInit
	if err := c.get(ctx, apiBase+"/room/v1/Room/room_init", q, &ri); err != nil {
		return nil, err
	}
	if ri.RoomID == 0 {
		return nil, fmt.Errorf("room %d not found", id)
	}
	return &ri, nil
}

// RoomInfo is the response from /room/v1/Room/get_info.
type RoomInfo struct {
	RoomID      int64           `json:"room_id"`
	ShortID     int64           `json:"short_id"`
	UID         int64           `json:"uid"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Online      int64           `json:"online"`
	LiveStatus  int             `json:"live_status"`
	LiveTime    json.RawMessage `json:"live_time"`
	AreaName    string          `json:"area_name"`
	ParentName  string          `json:"parent_name"`
	UserCover   string          `json:"user_cover"`
	Keyframe    string          `json:"keyframe"`
	Attention   int64           `json:"attention"`
}

// RoomInfo fetches room metadata. It also accepts short ids.
func (c *Client) RoomInfo(ctx context.Context, id int64) (*RoomInfo, error) {
	q := url.Values{"room_id": {strconv.FormatInt(id, 10)}}
	var ri RoomInfo
	if err := c.get(ctx, apiBase+"/room/v1/Room/get_info", q, &ri); err != nil {
		return nil, err
	}
	return &ri, nil
}

// DanmuInfo is the response from /xlive/web-room/v1/index/getDanmuInfo.
type DanmuInfo struct {
	Token    string      `json:"token"`
	HostList []DanmuHost `json:"host_list"`
}

// DanmuHost is one candidate danmaku server.
type DanmuHost struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	WSSPort int    `json:"wss_port"`
	WSPort  int    `json:"ws_port"`
}

// DanmuInfo fetches the danmaku server list and auth token for a full room id.
func (c *Client) DanmuInfo(ctx context.Context, roomID int64) (*DanmuInfo, error) {
	q := url.Values{
		"id":   {strconv.FormatInt(roomID, 10)},
		"type": {"0"},
	}
	// Since 2025-05 the endpoint requires WBI signing. Keep the unsigned
	// fallback for networks that block nav.
	signed, err := c.SignWBI(ctx, q)
	if err == nil {
		q = signed
	}
	var di DanmuInfo
	if err := c.get(ctx, apiBase+"/xlive/web-room/v1/index/getDanmuInfo", q, &di); err != nil {
		return nil, err
	}
	if di.Token == "" || len(di.HostList) == 0 {
		return nil, fmt.Errorf("no danmaku hosts for room %d", roomID)
	}
	return &di, nil
}

// NavUser holds the fields from /x/web-interface/nav that this client uses.
type NavUser struct {
	IsLogin bool   `json:"isLogin"`
	MID     int64  `json:"mid"`
	Uname   string `json:"uname"`
	Face    string `json:"face"`
}

// Nav checks the stored session with api.bilibili.com.
func (c *Client) Nav(ctx context.Context) (*NavUser, error) {
	var nav NavUser
	if err := c.get(ctx, apiMain+"/x/web-interface/nav", nil, &nav); err != nil {
		// Code -101 means the user is not logged in. Return it as a normal result.
		if apiErr, ok := err.(*APIError); ok && apiErr.Code == -101 {
			return &NavUser{IsLogin: false}, nil
		}
		return nil, err
	}
	return &nav, nil
}

// MasterRoomID returns the live room id for the given user id (mid).
// It returns 0 when the account has no live room.
func (c *Client) MasterRoomID(ctx context.Context, mid int64) (int64, error) {
	q := url.Values{"uid": {strconv.FormatInt(mid, 10)}}
	var master struct {
		RoomID int64 `json:"room_id"`
	}
	if err := c.get(ctx, apiBase+"/live_user/v1/Master/info", q, &master); err != nil {
		return 0, err
	}
	return master.RoomID, nil
}

// OwnRoomID returns the logged-in user's live room id via the cached
// account id or Nav. It requires a stored session.
func (c *Client) OwnRoomID(ctx context.Context) (int64, error) {
	if c.Session == nil || !c.Session.HasSession() {
		return 0, fmt.Errorf("%w: cannot resolve your own room without a stored session; run `bililive login` first", ErrLoginRequired)
	}
	mid := c.Session.DedeUserID
	if mid == 0 {
		nav, err := c.Nav(ctx)
		if err != nil {
			return 0, fmt.Errorf("lookup account id: %w", err)
		}
		if !nav.IsLogin || nav.MID == 0 {
			return 0, fmt.Errorf("%w: bilibili reports the stored session as logged out; run `bililive login` again", ErrLoginRequired)
		}
		mid = nav.MID
	}
	roomID, err := c.MasterRoomID(ctx, mid)
	if err != nil {
		return 0, fmt.Errorf("lookup own room: %w", err)
	}
	if roomID == 0 {
		return 0, fmt.Errorf("account mid %d has no live room; pass a room id to watch", mid)
	}
	return roomID, nil
}

// SendComment posts a danmaku comment to a room. It uses the stored session.
func (c *Client) SendComment(ctx context.Context, roomID int64, msg string, color int64) error {
	switch {
	case c.Session == nil || !c.Session.HasSession():
		return fmt.Errorf("not logged in: run `bililive login` first")
	case c.Session.BiliJCT == "":
		return fmt.Errorf("bili_jct (CSRF cookie) missing: re-run `bililive login` including bili_jct to enable sending")
	}
	form := url.Values{
		"bubble":     {"0"},
		"color":      {strconv.FormatInt(color, 10)},
		"fontsize":   {"25"},
		"mode":       {"1"},
		"msg":        {msg},
		"rnd":        {strconv.FormatInt(time.Now().Unix(), 10)},
		"roomid":     {strconv.FormatInt(roomID, 10)},
		"csrf":       {c.Session.CSRFToken()},
		"csrf_token": {c.Session.CSRFToken()},
	}
	var out struct {
		ModeInfo json.RawMessage `json:"mode_info"`
		DmV2     string          `json:"dm_v2"`
	}
	if err := c.postForm(ctx, apiBase+"/msg/send", form, &out); err != nil {
		return translateSendError(err)
	}
	return nil
}

func translateSendError(err error) error {
	apiErr, ok := err.(*APIError)
	if !ok {
		return err
	}
	switch apiErr.Code {
	case -101:
		return fmt.Errorf("session expired or not logged in (code -101): run `bililive login` again")
	case -111:
		return fmt.Errorf("csrf token mismatch (code -111): re-run `bililive login`")
	case 10031:
		return fmt.Errorf("sending too fast (code 10031), slow down")
	case 1003212:
		return fmt.Errorf("comment too long (code 1003212)")
	case -352, -424:
		return fmt.Errorf("risk control rejected the comment (code %d); try later or from the web", apiErr.Code)
	default:
		return apiErr
	}
}
