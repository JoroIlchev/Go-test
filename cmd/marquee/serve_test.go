package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"marquee/internal/auth"
	"marquee/internal/store"
)

func TestParseServe(t *testing.T) {
	for name, args := range map[string][]string{
		"stray word":               {"serve"},
		"public admin without TLS": {"-public-admin"},
		"cert without key":         {"-cert", "a.pem"},
		"domain and cert together": {"-domain", "x.bg", "-cert", "a", "-key", "b"},
	} {
		if _, err := parseServe(args); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c, err := parseServe([]string{"-domain", "ads.example.bg", "-public-admin"})
	if err != nil || c.publicURL != "https://ads.example.bg" {
		t.Fatalf("domain defaults: %v %+v", err, c)
	}
}

// TestHTTPS runs the hub with a self-signed certificate and checks what the
// internet sees: players on both HTTP and HTTPS, the admin only on HTTPS
// with a Secure cookie, and HTTP browsers sent to HTTPS.
func TestHTTPS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := selfSigned(t, dir)
	dataDir := filepath.Join(dir, "data")
	os.MkdirAll(dataDir, 0o750)
	st, err := store.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("correct-horse-battery")
	st.CreateUser(context.Background(), "root@hub", "Root", hash, true, nil, nil, nil)
	st.Close()

	httpsAddr, httpAddr := freeAddr(t), freeAddr(t)
	cfg, err := parseServe([]string{"-data", dataDir, "-addr", "", "-admin-addr", "",
		"-cert", certFile, "-key", keyFile, "-https-addr", httpsAddr, "-http-addr", httpAddr, "-public-admin"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil) }()
	defer func() {
		cancel()
		<-done
	}()

	client := &http.Client{
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}
	get := func(url string) *http.Response {
		var resp *http.Response
		var err error
		for i := 0; i < 50; i++ { // wait for the listeners
			if resp, err = client.Get(url); err == nil {
				return resp
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("GET %s: %v", url, err)
		return nil
	}

	resp := get("https://" + httpsAddr + "/")
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Strict-Transport-Security") == "" {
		t.Errorf("admin over HTTPS: %d hsts=%q", resp.StatusCode, resp.Header.Get("Strict-Transport-Security"))
	}
	if resp := get("https://" + httpsAddr + "/p/index.smil"); resp.StatusCode != 400 { // no player UUID
		t.Errorf("player over HTTPS: %d", resp.StatusCode)
	}
	if resp := get("http://" + httpAddr + "/p/index.smil"); resp.StatusCode != 400 {
		t.Errorf("player over HTTP: %d", resp.StatusCode)
	}
	resp = get("http://" + httpAddr + "/api/me")
	if resp.StatusCode != 301 || !strings.HasPrefix(resp.Header.Get("Location"), "https://") {
		t.Errorf("admin over HTTP: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	req, _ := http.NewRequest("POST", "https://"+httpsAddr+"/api/login",
		strings.NewReader(`{"email":"root@hub","password":"correct-horse-battery"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Marquee", "1")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if c := resp.Header.Get("Set-Cookie"); resp.StatusCode != 200 || !strings.Contains(c, "Secure") {
		t.Errorf("login over HTTPS: %d cookie=%q", resp.StatusCode, c)
	}
}

func freeAddr(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func selfSigned(t *testing.T, dir string) (certFile, keyFile string) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:   time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
	return certFile, keyFile
}
