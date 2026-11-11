package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/sync/errgroup"

	"marquee/internal/admin"
	"marquee/internal/logx"
	"marquee/internal/media"
	"marquee/internal/ops"
	"marquee/internal/player"
	"marquee/internal/poll"
	"marquee/internal/store"
	"marquee/web"
)

type config struct {
	addr, adminAddr, dataDir, publicURL, toolsDir, tz string
	maxUploadMB                                       int64
	refresh                                           time.Duration

	// Internet-facing HTTPS.
	domain, acmeEmail, certFile, keyFile, httpsAddr, httpAddr string
	publicAdmin                                               bool
}

func (c *config) https() bool { return c.domain != "" || c.certFile != "" }

func parseServe(args []string) (*config, error) {
	c := &config{}
	fl := flag.NewFlagSet("marquee", flag.ContinueOnError)
	fl.StringVar(&c.addr, "addr", ":8080", `plain-HTTP address for players ("" = off)`)
	fl.StringVar(&c.adminAddr, "admin-addr", "127.0.0.1:8081", "address of the admin UI on this machine (localhost only by default)")
	fl.StringVar(&c.dataDir, "data", defaultDataDir(), "directory for the database and media")
	fl.StringVar(&c.publicURL, "public-url", "", "address players use to reach this hub (default http://localhost:8080, or https://<domain>)")
	fl.StringVar(&c.toolsDir, "tools", "", `directory with ffmpeg.exe and ffprobe.exe (default: tools\ next to the exe, then PATH)`)
	fl.Int64Var(&c.maxUploadMB, "max-upload-mb", 4096, "largest accepted upload per file, in MB")
	fl.StringVar(&c.tz, "tz", "Europe/Sofia", "time zone of campaign schedules")
	fl.DurationVar(&c.refresh, "refresh", 15*time.Minute, "how often players check for changes unless a node sets its own schedule (min 1m)")

	fl.StringVar(&c.domain, "domain", "", "public domain name: serve HTTPS with a free Let's Encrypt certificate (needs ports 80 and 443 open)")
	fl.StringVar(&c.acmeEmail, "acme-email", "", "e-mail Let's Encrypt writes to about certificate problems")
	fl.StringVar(&c.certFile, "cert", "", "TLS certificate file (PEM), instead of Let's Encrypt")
	fl.StringVar(&c.keyFile, "key", "", "TLS private key file (PEM), with -cert")
	fl.StringVar(&c.httpsAddr, "https-addr", ":443", "HTTPS address with -domain or -cert")
	fl.StringVar(&c.httpAddr, "http-addr", ":80", "HTTP address with -domain or -cert: certificate checks, players, redirect to HTTPS")
	fl.BoolVar(&c.publicAdmin, "public-admin", false, "also serve the admin UI (login page) over HTTPS, reachable from the internet")
	if err := fl.Parse(args); err != nil {
		return nil, err
	}
	// Flag parsing stops at the first non-flag, so anything after a stray word
	// would be silently ignored, -data included.
	if fl.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument %q (flags only; see -h)", fl.Arg(0))
	}
	if (c.certFile == "") != (c.keyFile == "") {
		return nil, errors.New("-cert and -key go together")
	}
	if c.domain != "" && c.certFile != "" {
		return nil, errors.New("use either -domain (Let's Encrypt) or -cert/-key, not both")
	}
	if c.publicAdmin && !c.https() {
		return nil, errors.New("-public-admin needs HTTPS: add -domain (or -cert and -key)")
	}
	// Without -public-url the address comes from the settings or this
	// machine's IP; a domain fixes it.
	if c.publicURL == "" && c.domain != "" {
		c.publicURL = "https://" + c.domain
	}
	return c, nil
}

// playerEndpoint is how devices reach the players' listener: plain HTTP on
// -addr, else HTTPS.
func (c *config) playerEndpoint() (scheme, port string) {
	if c.addr != "" {
		return "http", admin.PlayerPortOf(c.addr)
	}
	return "https", admin.PlayerPortOf(c.httpsAddr)
}

func run(ctx context.Context, c *config, log *slog.Logger, logs *logx.DailyFile) error {
	loc, err := time.LoadLocation(c.tz)
	if err != nil {
		return fmt.Errorf("-tz: %w", err)
	}
	if err := os.MkdirAll(c.dataDir, 0o750); err != nil {
		return err
	}
	st, err := store.Open(ctx, c.dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	if n, err := st.CountUsers(ctx); err == nil && n == 0 {
		log.Warn("Още няма потребители: създайте първия с marquee.exe create-super -email vie@firma.bg")
	}

	storage, err := media.NewStorage(filepath.Join(c.dataDir, "media"))
	if err != nil {
		return err
	}
	tools := media.FindTools(c.toolsDir)
	if tools.FFmpeg == "" || tools.FFprobe == "" {
		log.Warn("Няма ffmpeg: видеата се качват, но не се проверяват и конвертират",
			"решение", `сложете ffmpeg.exe и ffprobe.exe в папка tools до marquee.exe`)
	} else {
		log.Info("ffmpeg е наличен", "път", tools.FFmpeg)
	}
	worker := media.NewWorker(st, storage, tools, log)
	worker.SetOptions(func(ctx context.Context) media.Options {
		set, err := ops.Load(ctx, st)
		if err != nil {
			return ops.Defaults().MediaOptions()
		}
		return set.MediaOptions()
	})
	monitor := ops.NewMonitor(st, log, loc)
	backup := ops.NewBackup(st, log, loc, c.dataDir)

	playerH := player.New(st, storage, log, player.Config{
		PublicURL: c.publicURL, Loc: loc, RefreshSec: int(c.refresh.Seconds()),
		DefaultPoll: func(ctx context.Context) poll.Policy {
			p, _ := ops.DefaultPoll(ctx, st, poll.Interval(c.refresh))
			return p
		},
	})
	scheme, port := c.playerEndpoint()
	adminCfg := admin.Config{
		PublicURL: c.publicURL, PlayerScheme: scheme, PlayerPort: port, Logs: logs,
		Web: web.Dist(), Storage: storage, Worker: worker,
		MaxUpload: c.maxUploadMB << 20, Loc: loc, DefaultPoll: poll.Interval(c.refresh), Backup: backup,
	}

	type server struct {
		*http.Server
		purpose string
	}
	var servers []server
	add := func(addr, purpose string, h http.Handler, tlsConf *tls.Config) {
		if addr != "" {
			servers = append(servers, server{&http.Server{Addr: addr, Handler: h, TLSConfig: tlsConf,
				ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}, purpose})
		}
	}
	add(c.addr, "устройства", playerH, nil)
	adminSrv := admin.New(st, log, adminCfg)
	add(c.adminAddr, "админ панел", adminSrv, nil)

	if c.https() {
		var publicAdmin http.Handler
		if c.publicAdmin {
			// Its own instance: the session cookie must carry the Secure flag here.
			secure := adminCfg
			secure.SecureCookie = true
			publicAdmin = admin.New(st, log, secure)
		}
		tlsConf, acmeHTTP, err := tlsSetup(c)
		if err != nil {
			return err
		}
		purpose := "устройства по HTTPS"
		if c.publicAdmin {
			purpose = "устройства и админ панел по HTTPS"
		}
		add(c.httpsAddr, purpose, hsts(public(playerH, publicAdmin)), tlsConf)
		add(c.httpAddr, "сертификат и пренасочване към HTTPS", acmeHTTP(public(playerH, redirectToHTTPS())), nil)
		log.Info("HTTPS е включен", "домейн", c.domain, "админ отвън", c.publicAdmin)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { worker.Run(gctx); return nil })
	g.Go(func() error { monitor.Run(gctx); return nil })
	g.Go(func() error { backup.Run(gctx); return nil })
	for _, srv := range servers {
		g.Go(func() error {
			log.Info("Слуша за "+srv.purpose, "адрес", srv.Addr)
			var err error
			if srv.TLSConfig != nil {
				err = srv.ListenAndServeTLS("", "")
			} else {
				err = srv.ListenAndServe()
			}
			if !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("%s: %w", srv.Addr, err)
			}
			return nil
		})
	}
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, srv := range servers {
			srv.Shutdown(shutdownCtx)
		}
		return nil
	})
	log.Info("marquee е стартиран", "данни", c.dataDir)
	hub := adminSrv.HubURL(ctx)
	if ops.IsPrivate(hub) {
		log.Warn("Адрес за устройствата е вътрешен", "адрес", hub, "решение", "публичният IP се задава в Настройки")
	} else {
		log.Info("Адрес за устройствата", "адрес", hub)
	}
	if c.adminAddr != "" {
		log.Info("Админ панел", "адрес", "http://"+strings.Replace(c.adminAddr, "0.0.0.0", "localhost", 1))
	}
	return g.Wait()
}

// tlsSetup returns the HTTPS config and a wrapper for the plain-HTTP side,
// which with Let's Encrypt must answer the certificate challenges.
func tlsSetup(c *config) (*tls.Config, func(http.Handler) http.Handler, error) {
	if c.certFile != "" {
		cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("-cert/-key: %w", err)
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
			func(h http.Handler) http.Handler { return h }, nil
	}
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(c.domain),
		Cache:      autocert.DirCache(filepath.Join(c.dataDir, "certs")),
		Email:      c.acmeEmail,
	}
	conf := m.TLSConfig()
	conf.MinVersion = tls.VersionTLS12 // Android 7 speaks TLS 1.2
	return conf, m.HTTPHandler, nil
}

// public routes the internet-facing listeners: player paths go to the player
// handler, everything else to rest (the admin UI, or a redirect), or 404.
func public(playerH, rest http.Handler) http.Handler {
	mux := http.NewServeMux()
	for _, p := range []string{"/m/", "/p/", "/d/"} {
		mux.Handle(p, playerH)
	}
	if rest != nil {
		mux.Handle("/", rest)
	} else {
		mux.Handle("/", http.NotFoundHandler())
	}
	return mux
}

// Players configured with an http:// address keep working; anything else
// (a browser opening the admin) moves to HTTPS.
func redirectToHTTPS() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[i:], "]") {
			host = host[:i]
		}
		http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusMovedPermanently)
	})
}

func hsts(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		h.ServeHTTP(w, r)
	})
}
