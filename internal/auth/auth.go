// Package auth holds password hashing, session tokens, login throttling and
// the permission rules of the organisation tree.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const MinPasswordLen = 10

var ErrWeakPassword = errors.New("password must be at least 10 characters")

func HashPassword(pw string) (string, error) {
	if utf8.RuneCountInString(pw) < MinPasswordLen {
		return "", ErrWeakPassword
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	return string(h), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// dummyHash keeps login timing the same whether or not the e-mail exists.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("marquee-timing-equaliser"), 12)

func BurnPasswordCheck(pw string) { bcrypt.CompareHashAndPassword(dummyHash, []byte(pw)) }

// NewSessionToken returns the cookie value and the hash that is stored in the database.
func NewSessionToken() (token, hash string) {
	b := make([]byte, 32)
	rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Limiter locks a key (an e-mail or an IP) for lockFor after maxFails failed
// logins within lockFor. State is in memory; a restart clears it, which is fine.
type Limiter struct {
	mu       sync.Mutex
	maxFails int
	lockFor  time.Duration
	entries  map[string]*limiterEntry
}

type limiterEntry struct {
	fails       int
	first       time.Time
	lockedUntil time.Time
}

func NewLimiter(maxFails int, lockFor time.Duration) *Limiter {
	return &Limiter{maxFails: maxFails, lockFor: lockFor, entries: map[string]*limiterEntry{}}
}

func (l *Limiter) Locked(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for _, k := range keys {
		if e := l.entries[strings.ToLower(k)]; e != nil && now.Before(e.lockedUntil) {
			return true
		}
	}
	return false
}

func (l *Limiter) Fail(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for _, k := range keys {
		k = strings.ToLower(k)
		e := l.entries[k]
		if e == nil || now.Sub(e.first) > l.lockFor {
			e = &limiterEntry{first: now}
			l.entries[k] = e
		}
		e.fails++
		if e.fails >= l.maxFails {
			e.lockedUntil = now.Add(l.lockFor)
		}
	}
	// Opportunistic cleanup so the map cannot grow without bound.
	if len(l.entries) > 10000 {
		for k, e := range l.entries {
			if now.Sub(e.first) > l.lockFor && now.After(e.lockedUntil) {
				delete(l.entries, k)
			}
		}
	}
}

func (l *Limiter) Reset(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		delete(l.entries, strings.ToLower(k))
	}
}
