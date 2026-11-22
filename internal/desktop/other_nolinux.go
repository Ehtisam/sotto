//go:build !windows && !linux

package desktop

import "time"

// Media: bu sistemde medya denetimi yok (macOS ve diğerleri).
type Media struct{}

func OpenMedia() (*Media, error)                      { return nil, ErrUnsupported }
func (m *Media) Events() <-chan Event                 { return nil }
func (m *Media) SetTrack(Track) error                 { return nil }
func (m *Media) SetStatus(Status) error               { return nil }
func (m *Media) SetPosition(_, _ time.Duration) error { return nil }
func (m *Media) Close()                               {}

// Menü kısayolu bu sistemde yok.
func InstallShortcut(string) error                    { return ErrUnsupported }
func RemoveShortcut() error                           { return nil }
func ShortcutInfo() (target, appID string, err error) { return "", "", ErrUnsupported }
func ShortcutOK(string) bool                          { return true }
