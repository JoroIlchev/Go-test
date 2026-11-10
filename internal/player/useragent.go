package player

import (
	"regexp"
	"strings"
)

// SMIL players send e.g.
//
//	GAPI/1.0 (UUID:7c9e...; NAME:screen-1) player/1.0 (MODEL:box)
//
// The browser (WASM) build cannot set User-Agent and uses X-Signage-Agent instead.
var (
	uaUUID = regexp.MustCompile(`UUID:\s*([0-9A-Za-z-]+)`)
	uaName = regexp.MustCompile(`NAME:\s*([^;)]*)`)
)

type agent struct {
	UUID string
	Name string
	Raw  string
}

func parseAgent(userAgent, signageAgent string) agent {
	raw := userAgent
	if !strings.Contains(raw, "UUID:") && signageAgent != "" {
		raw = signageAgent
	}
	a := agent{Raw: raw}
	if m := uaUUID.FindStringSubmatch(raw); m != nil {
		a.UUID = strings.ToLower(m[1])
	}
	if m := uaName.FindStringSubmatch(raw); m != nil {
		a.Name = strings.TrimSpace(m[1])
	}
	return a
}
