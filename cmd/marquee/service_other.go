//go:build !windows && !linux

package main

import "errors"

func isService() bool { return false }

func runService([]string) error { return errors.New("not a Windows service") }

func installService([]string) error {
	return errors.New("install-service is Windows and Linux only")
}

func uninstallService() error { return errors.New("uninstall-service is Windows and Linux only") }
