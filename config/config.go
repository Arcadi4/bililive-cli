// Package config stores bililive-cli settings: per-room preferences and
// bilibili session cookies.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Dir returns the config directory and creates it if needed.
// Preferences live here.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	dir := filepath.Join(base, "bililive-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return dir, nil
}

// CacheDir returns the system cache directory, creating it if needed.
// Session cookies live here, or in the config dir when no cache exists.
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		return Dir()
	}
	dir := filepath.Join(base, "bililive-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	return dir, nil
}

// Session holds the cookies for REST calls and live connections.
type Session struct {
	// SESSDATA is the main login cookie.
	SESSDATA string `yaml:"sessdata"`
	// BiliJCT is the CSRF token cookie. POST endpoints require it.
	BiliJCT string `yaml:"bili_jct"`
	// DedeUserID is cached account metadata. It feeds the WebSocket uid and the renderer.
	DedeUserID int64 `yaml:"dede_user_id"`
	// BUVID3 is the device identifier. Live connections require it.
	BUVID3 string `yaml:"buvid3"`
	// SavedAt is the time when the session was stored. It uses RFC3339 format.
	SavedAt string `yaml:"saved_at"`
}

// HasSession reports whether the session can watch with the account
// identity. The session needs SESSDATA. Sending comments also needs BiliJCT.
func (s *Session) HasSession() bool {
	return s.SESSDATA != ""
}

// LoggedIn reports whether the session can send comments.
func (s *Session) LoggedIn() bool {
	return s.HasSession() && s.BiliJCT != ""
}

// CookieHeader builds the session cookie header for bilibili endpoints.
func (s *Session) CookieHeader() string {
	var pairs []string
	if s.SESSDATA != "" {
		pairs = append(pairs, "SESSDATA="+s.SESSDATA)
	}
	if s.BiliJCT != "" {
		pairs = append(pairs, "bili_jct="+s.BiliJCT)
	}
	if s.BUVID3 != "" {
		pairs = append(pairs, "buvid3="+s.BUVID3)
	}
	return strings.Join(pairs, "; ")
}

func (s *Session) CSRFToken() string { return s.BiliJCT }

// LoadSession reads the stored session. It returns (nil, nil) when no
// session file exists.
func LoadSession() (*Session, error) {
	dir, err := CacheDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "session.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var s Session
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &s, nil
}

// SaveSession saves the session with 0600 permissions.
func SaveSession(s *Session) error {
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "session.yaml")
	tmp, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return fmt.Errorf("create session temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set session file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write session: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close session: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace session: %w", err)
	}
	return nil
}

// ClearSession removes the stored session. It returns nil when no session exists.
func ClearSession() error {
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, "session.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Preferences holds per-room display defaults. They persist in prefs.yaml.
type Preferences struct {
	// ShowEntrants shows INTERACT_WORD enter lines.
	ShowEntrants bool `yaml:"show_entrants"`
	// ShowLikes shows total like counts.
	ShowLikes bool `yaml:"show_likes"`
	// ShowCombo merges gift combo lines into the first gift line.
	ShowCombo bool `yaml:"show_combo"`
}

func DefaultPreferences() Preferences {
	return Preferences{
		ShowEntrants: true,
		ShowLikes:    true,
		ShowCombo:    true,
	}
}

// LoadPreferences reads stored preferences. A missing or invalid file
// returns the defaults.
func LoadPreferences() (Preferences, error) {
	p := DefaultPreferences()
	dir, err := Dir()
	if err != nil {
		return p, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "prefs.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("read prefs: %w", err)
	}
	if err := yaml.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("parse prefs: %w", err)
	}
	return p, nil
}

func SavePreferences(p Preferences) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "prefs.yaml")
	return os.WriteFile(path, data, 0o600)
}
