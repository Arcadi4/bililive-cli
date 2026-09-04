package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Arcadi4/bililive-cli/bili"
	"github.com/Arcadi4/bililive-cli/config"
	"github.com/charmbracelet/lipgloss"
)

// Renderer turns danmaku events into themed single-line log strings.
type Renderer struct {
	theme *Theme
	prefs config.Preferences
	icons Icons
	// SelfID is the logged-in user mid. It is 0 for anonymous users. Own
	// comments render in the Self style.
	SelfID int64
	// RoomID is the watched room. NOTICE_MSG broadcasts about other rooms
	// stay hidden unless ShowAll is set. These cover gifts, wishes, and
	// rank announcements.
	RoomID int64
	// ShowAll keeps unknown protocol messages visible as raw lines for --all.
	ShowAll bool
}

// NewRenderer builds a renderer from theme and prefs. unicode=true
// selects the plain unicode icons in place of the default nerd font icons.
func NewRenderer(t *Theme, prefs config.Preferences, unicode bool) *Renderer {
	icons := NerdIcons()
	if unicode {
		icons = UnicodeIcons()
	}
	return &Renderer{theme: t, prefs: prefs, icons: icons}
}

// Render formats a bilibili Event into one display line with no
// trailing newline. It returns false when user preferences skip
// the event.
func (r *Renderer) Render(ev bili.Event) (string, bool) {
	cmd := ev.Cmd
	// DANMU_MSG carries a version suffix such as DANMU_MSG:4:0:2:2:2:0.
	if i := strings.Index(cmd, ":"); i > 0 {
		cmd = cmd[:i]
	}

	body := map[string]any{}
	if len(ev.Raw) > 0 {
		if err := json.Unmarshal(ev.Raw, &body); err != nil {
			return r.raw(ev)
		}
	}

	switch cmd {
	case "DANMU_MSG":
		return r.danmu(body)
	case "SEND_GIFT":
		return r.gift(body, false)
	case "COMBO_SEND":
		return r.gift(body, true)
	case "GUARD_BUY":
		return r.guard(body)
	case "USER_TOAST_MSG", "USER_TOAST_MSG_V2":
		return r.userToast(body)
	case "SUPER_CHAT_MESSAGE":
		return r.superChat(body, false)
	case "SUPER_CHAT_MESSAGE_JPN":
		return r.superChat(body, true)
	case "SUPER_CHAT_MESSAGE_DELETE":
		return "", true
	case "INTERACT_WORD":
		return r.interact(body)
	case "ENTRY_EFFECT":
		return r.entryEffect(body)
	case "LIKE_INFO_V3_CLICK":
		return r.likeClick(body)
	case "LIKE_INFO_V3_UPDATE":
		return r.likeUpdate(body)
	case "WATCHED_CHANGE":
		return r.watched(body)
	case "ONLINE_RANK_COUNT":
		return r.onlineRank(body)
	case "ROOM_REAL_TIME_MESSAGE_UPDATE":
		return r.fansUpdate(body)
	case "NOTICE_MSG":
		return r.notice(body)
	case "ROOM_BLOCK_MSG":
		return r.block(body)
	case "LIVE":
		return r.line("SYS", r.icons.Play+" live started"), true
	case "PREPARING":
		return r.line("SYS", r.icons.Stop+" stream ended / preparing"), true
	case "CUT_OFF":
		return r.line("ERROR", r.icons.Warn+" stream cut off: "+str(body, "msg")), true
	case "STOP_LIVE_ROOM_LIST":
		return "", true
	case "LOG_IN_NOTICE":
		return r.line("SYS", "notice: log in for full user info (some users show as * / uid 0)"), true
	default:
		return r.raw(ev)
	}
}

func (r *Renderer) line(level, text string) string {
	return r.style(level).Render(text)
}

// RenderSelfComment renders an echo of a comment the user just sent.
func (r *Renderer) RenderSelfComment(text string) string {
	return r.line("SELF", "你: "+text)
}

// RenderSendError renders a comment-send failure.
func (r *Renderer) RenderSendError(err error) string {
	return r.line("ERROR", "发送失败: "+err.Error())
}

// RenderReconnect renders a connection-loss diagnostic.
func (r *Renderer) RenderReconnect(err error) string {
	return r.line("CONN", "连接断开，正在重连: "+err.Error())
}

func (r *Renderer) style(level string) lipgloss.Style {
	switch level {
	case "DANM":
		return r.theme.Danmaku
	case "GIFT":
		return r.theme.Gift
	case "GUARD":
		return r.theme.Guard
	case "SC":
		return r.theme.SuperChat
	case "SYS":
		return r.theme.Info
	case "ENTER":
		return r.theme.Enter
	case "LIKE":
		return r.theme.Like
	case "WISH":
		return r.theme.Notice
	case "SELF":
		return r.theme.Self
	case "RAW":
		return r.theme.BarDim
	case "ERROR":
		return r.theme.Error
	}
	return r.theme.Danmaku
}

func (r *Renderer) user(body map[string]any, uidKey, nameKey string) string {
	uid := int64(num(body[uidKey]))
	name := str(body, nameKey)
	return r.renderUser(uid, name, nil)
}

// renderUser renders a name with the fan medal and guard badges when
// present. medal is the parsed medal_info map, or nil. The fan medal and
// the guard badge render as two separate pills, not one bracketed segment.
func (r *Renderer) renderUser(uid int64, name string, medal map[string]any) string {
	var b strings.Builder
	b.WriteString(r.theme.Name.Render(name))
	if uid != 0 && r.SelfID != 0 && uid == r.SelfID {
		// Own actions keep the Self color for the name.
		b.Reset()
		b.WriteString(r.theme.Self.Render(name))
	}
	if medal != nil {
		mname := str(medal, "medal_name")
		lvl := int64(num(medal["medal_level"]))
		guard := int64(num(medal["guard_level"]))
		if mname != "" {
			b.WriteString(" ")
			b.WriteString(r.theme.Medal.Render(fmt.Sprintf("%s Lv%d", mname, lvl)))
		}
		if guard > 0 {
			b.WriteString(" ")
			b.WriteString(r.theme.GuardBadge.Render(guardName(guard)))
		}
	}
	return b.String()
}

// medalOf finds the fan-medal object. Most commands keep it on the body.
// SUPER_CHAT_MESSAGE variants nest it under user_info.
func medalOf(body map[string]any) map[string]any {
	if m, ok := body["medal_info"].(map[string]any); ok {
		return m
	}
	if ui, ok := body["user_info"].(map[string]any); ok {
		if m, ok := ui["medal_info"].(map[string]any); ok {
			return m
		}
	}
	return nil
}

// payloadOf returns the object that renderers read. It prefers the nested
// data object. Older wire formats keep the fields on the top level.
func payloadOf(body map[string]any) map[string]any {
	if d, ok := body["data"].(map[string]any); ok && len(d) > 0 {
		return d
	}
	return body
}

func (r *Renderer) danmu(body map[string]any) (string, bool) {
	info, ok := body["info"].([]any)
	if !ok || len(info) < 3 {
		return r.rawFromMap(body)
	}
	text := ""
	if len(info) > 1 {
		text = anyToString(info[1])
	}
	var uid, uname string
	var level int64
	if sender, ok := info[2].([]any); ok && len(sender) >= 2 {
		uid = anyToString(sender[0])
		uname = anyToString(sender[1])
		if len(sender) > 3 {
			level = int64(num(sender[3]))
		}
	}
	var medal map[string]any
	if len(info) > 3 {
		if m, ok := info[3].([]any); ok && len(m) >= 2 {
			medal = map[string]any{
				"medal_name":  anyToString(m[1]),
				"medal_level": num(m[0]),
				"guard_level": 0,
			}
			if len(m) > 10 {
				medal["guard_level"] = num(m[10])
			}
		}
	}
	name := r.renderUser(parseID(uid), uname, medal)
	ts := time.Now().Format("15:04:05")
	line := r.theme.BarDim.Render(ts+" ") + name
	if level > 0 {
		line += r.theme.BarDim.Render(fmt.Sprintf(" UL%d", level))
	}
	line += r.theme.BarDim.Render(": ") + r.theme.Danmaku.Render(text)
	return line, true
}

func (r *Renderer) gift(body map[string]any, combo bool) (string, bool) {
	d := payloadOf(body)
	uid := int64(num(d["uid"]))
	uname := str(d, "uname")
	gname := str(d, "giftName")
	if gname == "" {
		gname = str(d, "gift_name")
	}
	count := int64(num(d["num"]))
	if combo {
		count = int64(num(d["combo_num"]))
	}
	coin := int64(num(d["total_coin"]))
	if combo {
		coin = int64(num(d["combo_total_coin"]))
	}
	ctype := str(d, "coin_type")
	var price string
	if ctype == "silver" {
		price = fmt.Sprintf("%d银", coin/100)
	} else {
		price = fmt.Sprintf("¥%.1f", float64(coin)/1000)
	}
	guard := int64(num(d["guard_level"]))
	g := ""
	if guard > 0 {
		g = " · " + guardName(guard)
	}
	line := r.renderUser(uid, uname, medalOf(d)) +
		r.theme.Gift.Render(fmt.Sprintf(" 送出了 %s ×%d (%s%s)", gname, count, price, g))
	return line, true
}

func (r *Renderer) guard(body map[string]any) (string, bool) {
	d := payloadOf(body)
	uid := int64(num(d["uid"]))
	uname := str(d, "username")
	guard := int64(num(d["guard_level"]))
	count := int64(num(d["num"]))
	price := int64(num(d["price"]))
	line := r.renderUser(uid, uname, nil) +
		r.theme.Guard.Render(fmt.Sprintf(" 开通 %s ×%d (¥%.0f)",
			guardName(guard), count, float64(price)/1000*float64(count)))
	return line, true
}

func (r *Renderer) userToast(body map[string]any) (string, bool) {
	d := payloadOf(body)
	uname := str(d, "username")
	uid := int64(num(d["uid"]))
	role := str(d, "role_name")
	numN := int64(num(d["num"]))
	unit := str(d, "unit")
	if role == "" {
		role = guardName(int64(num(d["guard_level"])))
	}
	line := r.renderUser(uid, uname, nil) +
		r.theme.Guard.Render(fmt.Sprintf(" %s %s ×%d", role, unit, numN))
	return line, true
}

func (r *Renderer) superChat(body map[string]any, jpn bool) (string, bool) {
	d := payloadOf(body)
	uname := str(d, "user_info.uname")
	if uname == "" {
		uname = str(d, "uname")
	}
	uid := int64(num(d["uid"]))
	msg := str(d, "message")
	price := int64(num(d["price"]))
	ttl := int64(num(d["time"]))
	extra := ""
	if jpn {
		if j := str(d, "message_jpn"); j != "" && j != msg {
			extra = " / " + j
		}
	}
	line := r.renderUser(uid, uname, medalOf(d)) +
		r.theme.SuperChat.Render(fmt.Sprintf(" SC ¥%d (%ds): ", price, ttl)) +
		r.theme.Danmaku.Render(msg+extra)
	return line, true
}

func (r *Renderer) interact(body map[string]any) (string, bool) {
	if !r.prefs.ShowEntrants {
		return "", false
	}
	d := payloadOf(body)
	msgType := int64(num(d["msg_type"]))
	uid := int64(num(d["uid"]))
	uname := str(d, "uname")
	switch msgType {
	case 1:
		return r.renderUser(uid, uname, medalOf(d)) +
			r.theme.Enter.Render(" 进入直播间"), true
	case 2:
		return r.renderUser(uid, uname, medalOf(d)) +
			r.theme.Enter.Render(" 关注了主播"), true
	case 3:
		return r.renderUser(uid, uname, medalOf(d)) +
			r.theme.Enter.Render(" 分享了直播间"), true
	}
	return "", false
}

func (r *Renderer) entryEffect(body map[string]any) (string, bool) {
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return r.rawFromMap(body)
	}
	msg := strings.TrimSpace(stripTemplateMarkers(str(data, "copy_writing")))
	return r.theme.Enter.Render(msg), true
}

func (r *Renderer) likeClick(body map[string]any) (string, bool) {
	if !r.prefs.ShowLikes {
		return "", false
	}
	d := payloadOf(body)
	uid := int64(num(d["uid"]))
	uname := str(d, "uname")
	text := str(d, "like_text")
	if text == "" {
		text = "点赞"
	}
	return r.renderUser(uid, uname, medalOf(d)) +
		r.theme.Like.Render(" "+text), true
}

// likeUpdate renders total like counts for LIKE_INFO_V3_UPDATE. It is
// metadata, so --all shows it and --quiet still hides it.
func (r *Renderer) likeUpdate(body map[string]any) (string, bool) {
	if !r.prefs.ShowLikes || !r.ShowAll {
		return "", false
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return "", false
	}
	clicks := int64(num(data["click_count"]))
	return r.line("LIKE", fmt.Sprintf("%s %s 个点赞", r.icons.Hearts, FormatCount(clicks))), true
}

func (r *Renderer) watched(body map[string]any) (string, bool) {
	if !r.ShowAll {
		return "", false
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return "", false
	}
	return r.line("SYS", fmt.Sprintf("%s %s 人看过",
		r.icons.Watched, FormatCount(int64(num(data["num"]))))), true
}

func (r *Renderer) onlineRank(body map[string]any) (string, bool) {
	if !r.ShowAll {
		return "", false
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return "", false
	}
	return r.line("SYS", fmt.Sprintf("%s 高能榜 %s 人在线",
		r.icons.Bolt, FormatCount(int64(num(data["count"]))))), true
}

func (r *Renderer) fansUpdate(body map[string]any) (string, bool) {
	if !r.ShowAll {
		return "", false
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return "", false
	}
	fans := int64(num(data["fans"]))
	fansClub := int64(num(data["fans_club"]))
	return r.line("SYS", fmt.Sprintf("%s 粉丝 %s · 粉丝团 %s",
		r.icons.Fans, FormatCount(fans), FormatCount(fansClub))), true
}

func (r *Renderer) notice(body map[string]any) (string, bool) {
	// NOTICE_MSG also carries cross-room broadcasts. Hide messages from
	// other rooms unless --all is set. Messages without a room id show.
	if room := int64(num(body["real_roomid"])); room > 0 && room != r.RoomID && !r.ShowAll {
		return "", false
	}
	common := stripTemplateMarkers(str(body, "msg_common"))
	self := stripTemplateMarkers(str(body, "msg_self"))
	if self != "" {
		return r.line("WISH", self), true
	}
	if common != "" {
		return r.line("WISH", common), true
	}
	return r.rawFromMap(body)
}

func (r *Renderer) block(body map[string]any) (string, bool) {
	data, _ := body["data"].(map[string]any)
	if data == nil {
		data = body
	}
	uname := str(data, "uname")
	return r.line("ERROR", fmt.Sprintf("%s %s 已被房管/主播禁言", r.icons.Ban, uname)), true
}

// raw renders an unparsed or unknown protocol message. It shows only with
// ShowAll (--all).
func (r *Renderer) raw(ev bili.Event) (string, bool) {
	if !r.ShowAll {
		return "", false
	}
	return r.line("RAW", fmt.Sprintf("%s %s", ev.Cmd, truncateStr(string(ev.Raw), 160))), true
}

// rawFromMap renders a known command with a payload that failed to parse. It
// shows only with ShowAll (--all).
func (r *Renderer) rawFromMap(body map[string]any) (string, bool) {
	if !r.ShowAll {
		return "", false
	}
	cmd := str(body, "cmd")
	if i := strings.Index(cmd, ":"); i > 0 {
		cmd = cmd[:i]
	}
	raw, _ := json.Marshal(body)
	return r.line("RAW", fmt.Sprintf("%s %s", cmd, truncateStr(string(raw), 160))), true
}

func guardName(level int64) string {
	switch level {
	case 1:
		return "总督"
	case 2:
		return "提督"
	case 3:
		return "舰长"
	}
	return fmt.Sprintf("Guard%d", level)
}

func FormatCount(n int64) string {
	switch {
	case n >= 100_000_000:
		return fmt.Sprintf("%.1f亿", float64(n)/100_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1f万", float64(n)/10_000)
	default:
		return strconv.FormatInt(n, 10)
	}
}

func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	if v, ok := m[key]; ok {
		return anyToString(v)
	}
	// Dotted path lookup, for example user_info.uname.
	if strings.Contains(key, ".") {
		parts := strings.SplitN(key, ".", 2)
		if next, ok := m[parts[0]].(map[string]any); ok {
			return str(next, parts[1])
		}
	}
	return ""
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

func anyToString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case float64:
		if s == float64(int64(s)) {
			return strconv.FormatInt(int64(s), 10)
		}
		return strconv.FormatFloat(s, 'f', -1, 64)
	case json.Number:
		return s.String()
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func parseID(s string) int64 {
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// stripTemplateMarkers drops the <%...%> highlight wrappers that
// bilibili adds to template strings. A plain log keeps only the text.
// Never call it on user-typed content.
func stripTemplateMarkers(s string) string {
	if !strings.Contains(s, "<%") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for {
		i := strings.Index(s, "<%")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		s = s[i+2:]
		j := strings.Index(s, "%>")
		if j < 0 {
			// Unterminated wrapper. Keep the rest verbatim.
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:j])
		s = s[j+2:]
	}
}
