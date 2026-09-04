package bili

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// mixinKeyEncTab derives the WBI mixin key from the img and sub keys.
// See bilibili-API-collect ("wbi签名"), now mirrored at
// github.com/pskdje/bilibili-API-collect after a DMCA removal.
var mixinKeyEncTab = [64]int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49,
	33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40,
	61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11,
	36, 20, 34, 44, 52,
}

type wbiKeys struct {
	imgKey string
	subKey string
	fetch  time.Time
	mu     sync.Mutex
}

// SignWBI adds wts and w_rid to params. It signs them with the WBI scheme.
// getDanmuInfo and most live APIs require this since 2025-05.
func (c *Client) SignWBI(ctx context.Context, params url.Values) (url.Values, error) {
	img, sub, err := c.wbiKeyPair(ctx)
	if err != nil {
		return nil, err
	}
	mixin := mixinKey(img, sub)

	signed := url.Values{}
	for k, vs := range params {
		for _, v := range vs {
			signed.Add(k, v)
		}
	}
	signed.Set("wts", fmt.Sprintf("%d", time.Now().Unix()))

	// Encode sorted k=v pairs. Remove !'()* from values as the spec requires.
	var b strings.Builder
	for _, k := range sortedKeys(signed) {
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(k))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(strings.Map(func(r rune) rune {
			switch r {
			case '!', '\'', '(', ')', '*':
				return -1
			}
			return r
		}, signed.Get(k))))
	}
	sum := md5.Sum([]byte(b.String() + mixin))
	signed.Set("w_rid", hex.EncodeToString(sum[:]))
	return signed, nil
}

// wbiKeyPair fetches the img key and the sub key from the nav endpoint. It caches them.
func (c *Client) wbiKeyPair(ctx context.Context) (string, string, error) {
	if c.wbi == nil {
		c.wbi = &wbiKeys{}
	}
	c.wbi.mu.Lock()
	defer c.wbi.mu.Unlock()
	if c.wbi.imgKey != "" && time.Since(c.wbi.fetch) < 8*time.Hour {
		return c.wbi.imgKey, c.wbi.subKey, nil
	}
	var nav struct {
		WbiImg struct {
			ImgURL string `json:"img_url"`
			SubURL string `json:"sub_url"`
		} `json:"wbi_img"`
	}
	if err := c.get(ctx, apiMain+"/x/web-interface/nav", nil, &nav); err != nil {
		return "", "", fmt.Errorf("fetch wbi keys: %w", err)
	}
	img := keyFromURL(nav.WbiImg.ImgURL)
	sub := keyFromURL(nav.WbiImg.SubURL)
	if img == "" || sub == "" {
		return "", "", fmt.Errorf("empty wbi keys")
	}
	c.wbi.imgKey, c.wbi.subKey, c.wbi.fetch = img, sub, time.Now()
	return img, sub, nil
}

func keyFromURL(u string) string {
	i := strings.LastIndex(u, "/")
	j := strings.LastIndex(u, ".")
	if i < 0 || j <= i {
		return ""
	}
	return u[i+1 : j]
}

func mixinKey(img, sub string) string {
	src := img + sub
	var b strings.Builder
	for _, i := range mixinKeyEncTab {
		if i < len(src) {
			b.WriteByte(src[i])
		}
	}
	return b.String()[:32]
}

func sortedKeys(v url.Values) []string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
