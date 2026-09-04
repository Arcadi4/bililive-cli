package bili

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Arcadi4/bililive-cli/config"
)

const (
	qrLoginGenerateEndpoint = "https://passport.bilibili.com/x/passport-login/web/qrcode/generate"
	qrLoginPollEndpoint     = "https://passport.bilibili.com/x/passport-login/web/qrcode/poll"
	deviceIDEndpoint        = apiMain + "/x/frontend/finger/spi"
)

// QRLogin holds the data for a web login QR code.
type QRLogin struct {
	URL string
	Key string
}

// QRLoginState is the current state of a QR login attempt.
type QRLoginState string

const (
	QRLoginWaiting QRLoginState = "waiting"
	QRLoginScanned QRLoginState = "scanned"
	QRLoginExpired QRLoginState = "expired"
	QRLoginSuccess QRLoginState = "success"
)

// QRLoginStatus is the result of one QR login status poll.
type QRLoginStatus struct {
	State   QRLoginState
	Session *config.Session
}

// NewQRLogin starts a web QR login attempt. The code expires after three
// minutes. Check it with PollQRLogin.
func (c *Client) NewQRLogin(ctx context.Context) (*QRLogin, error) {
	var result struct {
		URL string `json:"url"`
		Key string `json:"qrcode_key"`
	}
	if err := c.get(ctx, qrLoginGenerateEndpoint, nil, &result); err != nil {
		return nil, fmt.Errorf("generate QR login: %w", err)
	}
	if result.URL == "" || result.Key == "" {
		return nil, fmt.Errorf("generate QR login: empty QR code data")
	}
	return &QRLogin{URL: result.URL, Key: result.Key}, nil
}

// PollQRLogin reports the state of a web QR login attempt. On success it
// returns only the cookies this client uses. The caller must verify the
// session before it saves the session.
func (c *Client) PollQRLogin(ctx context.Context, key string) (*QRLoginStatus, error) {
	requestURL := qrLoginPollEndpoint + "?" + url.Values{"qrcode_key": {key}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req, false)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll QR login: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read QR login status: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("QR login status: http %d", resp.StatusCode)
	}
	var envelope struct {
		APIError
		Data struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode QR login status: %w", err)
	}
	if envelope.Code != 0 {
		return nil, &APIError{Code: envelope.Code, Message: envelope.Message}
	}

	switch envelope.Data.Code {
	case 86101:
		return &QRLoginStatus{State: QRLoginWaiting}, nil
	case 86090:
		return &QRLoginStatus{State: QRLoginScanned}, nil
	case 86038:
		return &QRLoginStatus{State: QRLoginExpired}, nil
	case 0:
		session := sessionFromCookies(resp.Cookies())
		if !session.HasSession() || session.BiliJCT == "" {
			return nil, fmt.Errorf("QR login succeeded without required session cookies")
		}
		return &QRLoginStatus{State: QRLoginSuccess, Session: session}, nil
	default:
		return nil, fmt.Errorf("unexpected QR login status %d: %s", envelope.Data.Code, envelope.Data.Message)
	}
}

// DeviceID returns the buvid3 device cookie. Live connections require it.
func (c *Client) DeviceID(ctx context.Context) (string, error) {
	var result struct {
		BUVID3 string `json:"b_3"`
	}
	if err := c.get(ctx, deviceIDEndpoint, nil, &result); err != nil {
		return "", fmt.Errorf("fetch device ID: %w", err)
	}
	if result.BUVID3 == "" {
		return "", fmt.Errorf("fetch device ID: incomplete response")
	}
	return result.BUVID3, nil
}

func sessionFromCookies(cookies []*http.Cookie) *config.Session {
	session := &config.Session{}
	for _, cookie := range cookies {
		if cookie == nil || cookie.Value == "" {
			continue
		}
		switch cookie.Name {
		case "SESSDATA":
			session.SESSDATA = cookie.Value
		case "bili_jct":
			session.BiliJCT = cookie.Value
		case "DedeUserID":
			if mid, err := parseMID(cookie.Value); err == nil {
				session.DedeUserID = mid
			}
		}
	}
	return session
}

func parseMID(value string) (int64, error) {
	var mid int64
	if _, err := fmt.Sscan(strings.TrimSpace(value), &mid); err != nil {
		return 0, err
	}
	return mid, nil
}
