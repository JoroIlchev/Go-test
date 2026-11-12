//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
)

// On Linux systemd runs the hub as an ordinary foreground process, so there
// is no service mode inside the program; install-service writes the unit.

const unitPath = "/etc/systemd/system/marquee.service"

func isService() bool { return false }

func runService([]string) error { return errors.New("not a Windows service") }

// installService registers marquee with systemd to start on boot with the
// given flags and restart if it stops, then starts it. Run with sudo: the hub
// then runs as the user who called sudo, owning the data folder.
func installService(args []string) error {
	cfg, err := parseServe(args)
	if err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return errors.New("run with sudo: installing a service needs root")
	}
	for _, p := range []string{cfg.dataDir, cfg.toolsDir, cfg.certFile, cfg.keyFile} {
		if p != "" && !filepath.IsAbs(p) {
			return fmt.Errorf("use a full path for %q: the service does not start in this folder", p)
		}
	}
	if _, err := os.Stat(unitPath); err == nil {
		return errors.New("the service is already installed; run uninstall-service first to change its settings")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	name := os.Getenv("SUDO_USER")
	if name == "root" {
		name = ""
	}
	if name != "" {
		// The data folder may already exist from a run as root.
		if err := os.MkdirAll(cfg.dataDir, 0o750); err != nil {
			return err
		}
		if err := chownTree(cfg.dataDir, name); err != nil {
			return err
		}
	}
	unit := systemdUnit(exe, filepath.Dir(exe), name, args)
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return err
	}
	for _, c := range [][]string{{"daemon-reload"}, {"enable", "--now", "marquee"}} {
		if out, err := exec.Command("systemctl", c...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %v: %v %s", c, err, out)
		}
	}
	who := name
	if who == "" {
		who = "root"
	}
	fmt.Printf("service \"marquee\" installed and started as %s; logs: %s (or journalctl -u marquee)\n",
		who, filepath.Join(cfg.dataDir, "logs"))
	fmt.Println("open the player ports in the firewall if one is on, e.g. sudo ufw allow 80,443/tcp")
	return nil
}

func uninstallService() error {
	if os.Geteuid() != 0 {
		return errors.New("run with sudo: removing a service needs root")
	}
	if _, err := os.Stat(unitPath); err != nil {
		return errors.New("the service is not installed")
	}
	exec.Command("systemctl", "disable", "--now", "marquee").Run()
	if err := os.Remove(unitPath); err != nil {
		return err
	}
	exec.Command("systemctl", "daemon-reload").Run()
	fmt.Println(`service "marquee" removed (data is kept)`)
	return nil
}

func chownTree(dir, name string) error {
	u, err := user.Lookup(name)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return filepath.Walk(dir, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(p, uid, gid)
	})
}
