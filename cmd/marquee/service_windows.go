//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"

	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"marquee/internal/logx"
)

const serviceName = "marquee"

func isService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

// runService runs the hub under the Service Control Manager, logging to
// data\logs (a service has no console).
func runService(args []string) error {
	return svc.Run(serviceName, &service{args: args})
}

type service struct{ args []string }

func (s *service) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	cfg, err := parseServe(s.args)
	if err != nil {
		return true, 1
	}
	logs, err := openLogs(cfg)
	if err != nil {
		return true, 2
	}
	defer logs.Close()
	log := slog.New(logx.New(logs, slog.LevelInfo))
	log.Info("Услугата стартира")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, log, logs) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(20 * time.Second):
				}
				log.Info("Услугата е спряна")
				return false, 0
			}
		case err := <-done:
			if err != nil {
				log.Error("Услугата спря с грешка", "грешка", err)
				return true, 3 // non-zero: the recovery actions restart the service
			}
			return false, 0
		}
	}
}

// installService registers marquee to start with Windows, with the given
// flags, restarts it if it crashes, opens the firewall for it and starts it.
func installService(args []string) error {
	cfg, err := parseServe(args)
	if err != nil {
		return err
	}
	for _, p := range []string{cfg.dataDir, cfg.toolsDir, cfg.certFile, cfg.keyFile} {
		// The service starts in C:\Windows\System32.
		if p != "" && !filepath.IsAbs(p) {
			return fmt.Errorf("use a full path for %q: the service does not start in this folder", p)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("%w (run PowerShell as Administrator)", err)
	}
	defer m.Disconnect()
	if s, err := m.OpenService(serviceName); err == nil {
		s.Close()
		return errors.New("the service is already installed; run uninstall-service first to change its settings")
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName: "marquee",
		Description: "marquee digital signage hub",
		StartType:   mgr.StartAutomatic,
	}, args...)
	if err != nil {
		return err
	}
	defer s.Close()
	restart := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 10 * time.Second}
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{restart, restart, restart}, 24*60*60); err != nil {
		fmt.Println("warning: could not set restart on failure:", err)
	}
	// Players must reach the hub from the network.
	fw := exec.Command("netsh", "advfirewall", "firewall", "add", "rule", "name=marquee", "dir=in",
		"action=allow", "program="+exe, "enable=yes")
	if out, err := fw.CombinedOutput(); err != nil {
		fmt.Printf("warning: firewall rule not added: %v %s\n", err, out)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("installed, but did not start: %w", err)
	}
	fmt.Printf("service %q installed and started; logs: %s\n", serviceName, filepath.Join(cfg.dataDir, "logs"))
	return nil
}

func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("%w (run PowerShell as Administrator)", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return errors.New("the service is not installed")
	}
	defer s.Close()
	if st, err := s.Query(); err == nil && st.State != svc.Stopped {
		s.Control(svc.Stop)
		for i := 0; i < 30; i++ {
			if st, err := s.Query(); err != nil || st.State == svc.Stopped {
				break
			}
			time.Sleep(time.Second)
		}
	}
	if err := s.Delete(); err != nil {
		return err
	}
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=marquee").Run()
	fmt.Printf("service %q removed (data is kept)\n", serviceName)
	return nil
}
