package main

import (
	"fmt"
	"strings"
)

// systemdUnit is the unit file install-service writes on Linux. user empty
// means root; otherwise the hub runs as that user and may still bind ports 80
// and 443 for HTTPS.
func systemdUnit(exe, workDir, user string, args []string) string {
	cmd := []string{systemdQuote(exe)}
	for _, a := range args {
		cmd = append(cmd, systemdQuote(a))
	}
	var b strings.Builder
	b.WriteString("[Unit]\nDescription=marquee digital signage hub\n")
	b.WriteString("After=network-online.target\nWants=network-online.target\n\n")
	b.WriteString("[Service]\n")
	fmt.Fprintf(&b, "ExecStart=%s\n", strings.Join(cmd, " "))
	fmt.Fprintf(&b, "WorkingDirectory=%s\n", systemdQuote(workDir))
	if user != "" {
		fmt.Fprintf(&b, "User=%s\n", user)
		b.WriteString("AmbientCapabilities=CAP_NET_BIND_SERVICE\n")
	}
	b.WriteString("Restart=always\nRestartSec=10\n")
	// The hub finishes requests and closes the database on SIGTERM.
	b.WriteString("TimeoutStopSec=30\nLimitNOFILE=65536\n\n")
	b.WriteString("[Install]\nWantedBy=multi-user.target\n")
	return b.String()
}

// systemdQuote quotes one word of ExecStart: systemd expands % specifiers
// and $ variables, and splits on spaces.
func systemdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%", "$", "$$")
	return `"` + r.Replace(s) + `"`
}
