// Package i18n translates what the hub writes on its own: logs, e-mails,
// CSV exports and the pages players show. All of it follows one hub language
// (Settings.Lang, Bulgarian by default); the admin translates itself in the
// browser. Texts stay in Bulgarian in the code and T looks up the English
// one by that text, falling back to Bulgarian.
package i18n

import (
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	hub atomic.Value // string
	mu  sync.RWMutex
	en  = map[string]string{}
)

func init() { hub.Store("bg") }

// Hub is the hub language: "bg" or "en".
func Hub() string { return hub.Load().(string) }

// SetHub switches the hub language; anything but "en" means Bulgarian.
func SetHub(lang string) {
	if lang != "en" {
		lang = "bg"
	}
	hub.Store(lang)
}

// Register adds English texts keyed by the Bulgarian ones; packages call it
// from init with their own texts.
func Register(m map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	for k, v := range m {
		en[k] = v
	}
}

// T is bg in the hub language.
func T(bg string) string { return In(Hub(), bg) }

// Tf is T followed by fmt.Sprintf.
func Tf(bg string, args ...any) string { return fmt.Sprintf(T(bg), args...) }

// In is bg in the given language.
func In(lang, bg string) string {
	if lang != "en" {
		return bg
	}
	mu.RLock()
	defer mu.RUnlock()
	if s, ok := en[bg]; ok {
		return s
	}
	return bg
}
