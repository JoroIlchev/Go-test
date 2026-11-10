//go:build !windows

package main

import "errors"

func isService() bool { return false }

func runService([]string) error { return errors.New("services are Windows-only") }

func installService([]string) error {
	return errors.New("install-service is Windows-only; use systemd elsewhere")
}

func uninstallService() error { return errors.New("uninstall-service is Windows-only") }
