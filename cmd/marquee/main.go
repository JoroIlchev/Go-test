package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	_ "time/tzdata" // Windows has no IANA zone database

	"golang.org/x/term"

	"marquee/internal/auth"
	"marquee/internal/logx"
	"marquee/internal/store"
)

func main() {
	log := slog.New(logx.New(os.Stdout, slog.LevelInfo))
	var err error
	switch {
	case isService():
		err = runService(os.Args[1:])
	case len(os.Args) > 1 && os.Args[1] == "create-super":
		err = createSuper(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "install-service":
		err = installService(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "uninstall-service":
		err = uninstallService()
	default:
		var cfg *config
		var logs *logx.DailyFile
		if cfg, err = parseServe(os.Args[1:]); err == nil {
			if logs, err = openLogs(cfg); err == nil {
				defer logs.Close()
				// The console and the day's file get the same lines.
				log = slog.New(logx.New(logx.Fanout{os.Stdout, logs}, slog.LevelInfo))
				ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
				defer stop()
				err = run(ctx, cfg, log, logs)
			}
		}
	}
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		log.Error("Спиране заради грешка", "грешка", err)
		os.Exit(1)
	}
}

// Log files are kept for 90 days.
func openLogs(c *config) (*logx.DailyFile, error) {
	return logx.NewDailyFile(filepath.Join(c.dataDir, "logs"), 90)
}

// defaultDataDir is "data" next to the executable, not the working directory:
// a Windows service starts in C:\Windows\System32.
func defaultDataDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "data"
	}
	return filepath.Join(filepath.Dir(exe), "data")
}

func createSuper(args []string) error {
	fl := flag.NewFlagSet("create-super", flag.ExitOnError)
	email := fl.String("email", "", "e-mail of the new super admin")
	name := fl.String("name", "", "display name")
	dataDir := fl.String("data", defaultDataDir(), "directory for the database and media")
	fl.Parse(args)
	if *email == "" {
		return errors.New("-email is required")
	}
	pw, err := readPassword()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := os.MkdirAll(*dataDir, 0o750); err != nil {
		return err
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	u, err := st.CreateUser(ctx, *email, *name, hash, true, nil, nil, nil)
	if err != nil {
		return err
	}
	fmt.Printf("super admin %s created (id %d)\n", u.Email, u.ID)
	return nil
}

// readPassword prompts twice without echo on a terminal; piped input is read as one line.
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		// Windows PowerShell 5.1 prefixes piped text with a UTF-8 BOM.
		return strings.TrimPrefix(strings.TrimRight(line, "\r\n"), string(rune(0xFEFF))), nil
	}
	fmt.Print("Password: ")
	a, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	fmt.Print("Repeat password: ")
	b, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	if string(a) != string(b) {
		return "", errors.New("passwords do not match")
	}
	return string(a), nil
}
