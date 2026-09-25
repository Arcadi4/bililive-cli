package bili

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Arcadi4/bililive-cli/config"
	"github.com/andybalholm/brotli"
	"github.com/gorilla/websocket"
)

// Frame header layout. Each header has 16 bytes in big endian order.
//
//	[0:4] is the packet length. It includes the header.
//	[4:6] is the header length. It is always 16.
//	[6:8] is the protocol version. 0 is plain. 1 is auth and heartbeat reply. 2 is zlib. 3 is brotli.
//	[8:12] is the operation code.
//	[12:16] is the sequence number. It is always 1.
const (
	headerLen  = 16
	maxBody    = 8 << 20 // limit for the decompressed body size
	hbInterval = 30 * time.Second
	authExpiry = 5 * time.Second
)

// Frame protocol versions.
const (
	protoPlain  uint16 = 0
	protoAuth   uint16 = 1
	protoZlib   uint16 = 2
	protoBrotli uint16 = 3
)

// Operation codes.
const (
	opHeartbeat = 2
	opHbReply   = 3
	opMessage   = 5
	opAuth      = 7
	opAuthReply = 8
)

// Event is one decoded danmaku command.
type Event struct {
	// Cmd holds the command name, for example "DANMU_MSG".
	Cmd string          `json:"cmd"`
	Raw json.RawMessage `json:"-"`
}

// DanmakuClient streams bilibili live messages with backoff reconnects.
// Handlers run on the read goroutine, so each must return fast or hand
// off to another goroutine.
type DanmakuClient struct {
	RoomID int64
	// UID is the logged-in mid. It is 0 for anonymous connections.
	UID int64
	// Session is the stored session. It can be nil for anonymous use.
	Session *config.Session
	// Logger receives protocol diagnostics.
	Logger  *slog.Logger
	OnEvent func(Event)
	// OnStatus receives connection state changes. connected=false with
	// err=nil means a clean shutdown.
	OnStatus func(connected bool, err error)
	// OnPopularity receives room popularity values from heartbeats.
	OnPopularity func(popularity int64)
	client       *http.Client
	authOK       chan struct{}
	authFail     chan error
}

// Run connects to the danmaku stream. It blocks until ctx ends.
// It reconnects with capped exponential backoff.
func (d *DanmakuClient) Run(ctx context.Context) error {
	if d.OnEvent == nil {
		return fmt.Errorf("OnEvent handler required")
	}
	if d.client == nil {
		d.client = &http.Client{Timeout: 15 * time.Second}
	}
	backoff := time.Second
	if d.Session == nil || d.Session.SESSDATA == "" {
		return fmt.Errorf("%w: bilibili rejects anonymous danmaku connections (silent close since 2025); run `bililive login` first", ErrLoginRequired)
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := d.connectOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.OnStatus != nil {
			d.OnStatus(false, err)
		}
		if err == nil {
			backoff = time.Second
			continue
		}
		d.log("reconnecting", "error", err, "backoff", backoff.String())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (d *DanmakuClient) log(msg string, args ...any) {
	if d.Logger != nil {
		d.Logger.Info(msg, args...)
	}
}

// ErrLoginRequired reports a configuration problem. Do not retry it.
var ErrLoginRequired = errors.New("login required")

// connectOnce finds servers. It then dials, authenticates, and reads frames.
func (d *DanmakuClient) connectOnce(ctx context.Context) error {
	client := NewClient(d.Session)
	client.HTTP = d.client
	di, err := client.DanmuInfo(ctx, d.RoomID)
	if err != nil {
		return fmt.Errorf("getDanmuInfo: %w", err)
	}

	// Prefer wss_port. If no port is available, use broadcastlv.chat.bilibili.com.
	wsURL := ""
	for _, h := range di.HostList {
		port := h.WSSPort
		if port == 0 {
			port = h.Port
		}
		if port == 0 {
			continue
		}
		wsURL = fmt.Sprintf("wss://%s:%d/sub", h.Host, port)
		break
	}
	if wsURL == "" {
		wsURL = "wss://broadcastlv.chat.bilibili.com/sub"
	}
	// Auth sends a token plus the session cookie header like the web
	// client. Since 2025, servers close anonymous handshakes with no reply.
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	hdr := http.Header{}
	hdr.Set("User-Agent", userAgent)
	hdr.Set("Origin", "https://live.bilibili.com")
	if s := d.Session; s != nil {
		if v := s.CookieHeader(); v != "" {
			hdr.Set("Cookie", v)
		}
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, hdr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}
	defer conn.Close()

	authOK := make(chan struct{})
	authFail := make(chan error, 1)
	d.authOK, d.authFail = authOK, authFail

	if err := d.auth(conn, di.Token); err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	authDone := make(chan error, 1)
	go func() { authDone <- d.readLoop(ctx, conn) }()

	// Wait for the op-8 auth reply. Then start heartbeats.
	select {
	case err := <-authFail:
		return fmt.Errorf("auth rejected: %w", err)
	case <-authOK:
	case err := <-authDone:
		return err
	case <-time.After(authExpiry):
		conn.Close()
		<-authDone
		return fmt.Errorf("auth timeout")
	}

	if d.OnStatus != nil {
		d.OnStatus(true, nil)
	}

	// Send the first heartbeat immediately after the auth reply. The
	// ticker repeats it at hbInterval.
	if err := d.heartbeat(conn); err != nil {
		conn.Close()
		<-authDone
		return fmt.Errorf("initial heartbeat: %w", err)
	}
	tick := time.NewTicker(hbInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			conn.Close()
			<-authDone
			return nil
		case err := <-authDone:
			return err
		case <-tick.C:
			if err := d.heartbeat(conn); err != nil {
				conn.Close()
				<-authDone
				return fmt.Errorf("heartbeat: %w", err)
			}
		}
	}
}

func (d *DanmakuClient) auth(conn *websocket.Conn, token string) error {
	body, err := json.Marshal(map[string]any{
		"uid":      d.UID,
		"roomid":   d.RoomID,
		"protover": 3,
		"platform": "web",
		"type":     2,
		"key":      token,
		"buvid":    d.sessionBUVID(),
	})
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, buildPacket(protoAuth, opAuth, body))
}

func (d *DanmakuClient) sessionBUVID() string {
	if d.Session != nil {
		return d.Session.BUVID3
	}
	return ""
}

func (d *DanmakuClient) heartbeat(conn *websocket.Conn) error {
	return conn.WriteMessage(websocket.BinaryMessage,
		buildPacket(protoAuth, opHeartbeat, []byte("[object Object]")))
}

func (d *DanmakuClient) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if err := d.handleFrame(data); err != nil {
			d.log("frame error", "error", err)
		}
	}
}

func (d *DanmakuClient) handleFrame(data []byte) error {
	for len(data) >= headerLen {
		total := int(binary.BigEndian.Uint32(data[0:4]))
		if total < headerLen || total > len(data) {
			return fmt.Errorf("bad packet length %d (buffer %d)", total, len(data))
		}
		packet := data[:total]
		data = data[total:]

		proto := binary.BigEndian.Uint16(packet[6:8])
		op := binary.BigEndian.Uint32(packet[8:12])
		body := packet[headerLen:]

		switch op {
		case opAuthReply:
			var code struct {
				Code int `json:"code"`
			}
			if len(body) > 0 {
				if err := json.Unmarshal(body, &code); err != nil || code.Code != 0 {
					if d.authFail != nil {
						select {
						case d.authFail <- fmt.Errorf("code=%d body=%s", code.Code, truncate(string(body), 120)):
						default:
						}
					}
					return fmt.Errorf("auth rejected: %s", truncate(string(body), 120))
				}
			}
			if d.authOK != nil {
				select {
				case <-d.authOK:
				default:
					close(d.authOK)
				}
			}
			d.log("auth accepted")
		case opHbReply:
			if len(body) >= 4 {
				pop := int64(binary.BigEndian.Uint32(body[:4]))
				if d.OnPopularity != nil {
					d.OnPopularity(pop)
				}
			}

		case opMessage:
			if err := d.dispatchBody(proto, body); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *DanmakuClient) dispatchBody(proto uint16, body []byte) error {
	switch proto {
	case protoPlain, protoAuth:
		d.emit(body)
	case protoZlib, protoBrotli:
		var inflated []byte
		var err error
		switch proto {
		case protoZlib:
			inflated, err = inflateZlib(body)
		case protoBrotli:
			inflated, err = inflateBrotli(body)
		}
		if err != nil {
			return fmt.Errorf("decompress: %w", err)
		}
		// The inflated payload holds framed subpackets joined together.
		for len(inflated) >= headerLen {
			total := int(binary.BigEndian.Uint32(inflated[0:4]))
			if total < headerLen || total > len(inflated) {
				return fmt.Errorf("bad subpacket length %d", total)
			}
			sub := inflated[:total]
			inflated = inflated[total:]
			if subProto := binary.BigEndian.Uint16(sub[6:8]); subProto == protoZlib || subProto == protoBrotli {
				if err := d.dispatchBody(subProto, sub[headerLen:]); err != nil {
					return err
				}
				continue
			}
			d.emit(sub[headerLen:])
		}
	}
	return nil
}

func (d *DanmakuClient) emit(body []byte) {
	if len(body) == 0 {
		return
	}
	var probe struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		d.log("undecodable event", "body", truncate(string(body), 160))
		return
	}
	if d.Logger != nil {
		d.Logger.Debug("event", "cmd", probe.Cmd, "body", truncate(string(body), 1200))
	}
	d.OnEvent(Event{Cmd: probe.Cmd, Raw: append([]byte(nil), body...)})
}

func buildPacket(proto uint16, op uint32, body []byte) []byte {
	total := headerLen + len(body)
	packet := make([]byte, total)
	binary.BigEndian.PutUint32(packet[0:4], uint32(total))
	binary.BigEndian.PutUint16(packet[4:6], headerLen)
	binary.BigEndian.PutUint16(packet[6:8], proto)
	binary.BigEndian.PutUint32(packet[8:12], op)
	binary.BigEndian.PutUint32(packet[12:16], 1)
	copy(packet[headerLen:], body)
	return packet
}

func inflateZlib(body []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, maxBody))
}

func inflateBrotli(body []byte) ([]byte, error) {
	r := brotli.NewReader(bytes.NewReader(body))
	return io.ReadAll(io.LimitReader(r, maxBody))
}
