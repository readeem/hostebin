package cli

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hostebin/hostebin/internal/listen"
	"github.com/hostebin/hostebin/internal/server"
	"github.com/hostebin/hostebin/internal/store"
)

func runServe(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var addr, dataDir, baseURL, tlsAddr, tlsCert, tlsKey, acmeDomain, acmeEmail, tsHostname string
	var tailscale, funnel bool
	fs.StringVar(&addr, "addr", envDefault("HOSTEBIN_ADDR", ":8080"), "plain HTTP listen address")
	fs.StringVar(&dataDir, "data", envDefault("HOSTEBIN_DATA", "./data"), "data directory")
	fs.StringVar(&baseURL, "base-url", os.Getenv("HOSTEBIN_BASE_URL"), "public base URL override")
	fs.StringVar(&tlsAddr, "tls-addr", envDefault("HOSTEBIN_TLS_ADDR", ":8443"), "certificate TLS listen address")
	fs.StringVar(&tlsCert, "tls-cert", os.Getenv("HOSTEBIN_TLS_CERT"), "TLS certificate file")
	fs.StringVar(&tlsKey, "tls-key", os.Getenv("HOSTEBIN_TLS_KEY"), "TLS private key file")
	fs.StringVar(&acmeDomain, "acme-domain", os.Getenv("HOSTEBIN_ACME_DOMAIN"), "Let's Encrypt domain")
	fs.StringVar(&acmeEmail, "acme-email", os.Getenv("HOSTEBIN_ACME_EMAIL"), "Let's Encrypt account email")
	fs.BoolVar(&tailscale, "tailscale", envBool("HOSTEBIN_TS"), "enable embedded Tailscale")
	fs.BoolVar(&funnel, "funnel", envBool("HOSTEBIN_FUNNEL"), "enable Tailscale Funnel")
	fs.StringVar(&tsHostname, "ts-hostname", envDefault("HOSTEBIN_TS_HOSTNAME", "hostebin"), "Tailscale node hostname")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: hostebin serve [flags]")
		return exitUsage
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	st, err := store.New(dataDir)
	if err != nil {
		logger.Error("initialize storage", "error", err)
		return exitNetwork
	}
	token, generated, err := loadOrCreateToken(st.DataDir())
	if err != nil {
		logger.Error("initialize token", "error", err)
		return exitNetwork
	}
	if generated {
		logger.Info("generated upload token", "token", token, "path", filepath.Join(st.DataDir(), "token"))
	}
	maxUpload, err := parseBytes(envDefault("HOSTEBIN_MAX_UPLOAD", "32MiB"))
	if err != nil {
		logger.Error("invalid HOSTEBIN_MAX_UPLOAD", "error", err)
		return exitUsage
	}
	maxFiles, err := strconv.Atoi(envDefault("HOSTEBIN_MAX_FILES", "64"))
	if err != nil || maxFiles <= 0 {
		logger.Error("HOSTEBIN_MAX_FILES must be positive")
		return exitUsage
	}
	var defaultTTL time.Duration
	if raw := os.Getenv("HOSTEBIN_DEFAULT_TTL"); raw != "" && !strings.EqualFold(raw, "never") {
		defaultTTL, err = server.ParseDuration(raw)
		if err != nil || defaultTTL <= 0 {
			logger.Error("invalid HOSTEBIN_DEFAULT_TTL")
			return exitUsage
		}
	}
	csp := os.Getenv("HOSTEBIN_CSP")
	if csp == "" {
		csp = server.DefaultCSP
	}
	app, err := server.New(server.Config{Store: st, Token: token, MaxUpload: maxUpload, MaxFiles: maxFiles, DefaultTTL: defaultTTL, CSP: csp, Logger: logger})
	if err != nil {
		logger.Error("initialize server", "error", err)
		return exitNetwork
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	listeners, err := listen.Build(ctx, listen.Config{Addr: addr, BaseURL: baseURL, TLSAddr: tlsAddr, TLSCert: tlsCert, TLSKey: tlsKey, ACMEDomain: acmeDomain, ACMEEmail: acmeEmail, DataDir: st.DataDir(), Tailscale: tailscale || funnel, Funnel: funnel, TSHostname: tsHostname, TSAuthKey: os.Getenv("TS_AUTHKEY"), Logf: func(format string, values ...any) { logger.Info(fmt.Sprintf(format, values...)) }})
	if err != nil {
		logger.Error("initialize listeners", "error", err)
		return exitNetwork
	}
	defer listeners.Close()
	gcStop := make(chan struct{})
	defer close(gcStop)
	go st.RunGC(gcStop, 10*time.Minute, func(n int, err error) {
		if err != nil {
			logger.Error("garbage collection", "error", err)
		} else if n > 0 {
			logger.Info("removed expired bundles", "count", n)
		}
	})
	errCh := make(chan error, len(listeners.Endpoints))
	servers := make([]*http.Server, 0, len(listeners.Endpoints))
	for _, endpoint := range listeners.Endpoints {
		handler := endpoint.Handler
		if handler == nil {
			handler = app.Handler()
			if endpoint.BaseURL != "" {
				handler = server.WithBaseURL(endpoint.BaseURL, handler)
			}
		}
		httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
		servers = append(servers, httpServer)
		logger.Info("listening", "address", endpoint.Listener.Addr().String(), "base_url", endpoint.BaseURL)
		go func(ln net.Listener) { errCh <- httpServer.Serve(ln) }(endpoint.Listener)
	}
	select {
	case <-ctx.Done():
		shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, srv := range servers {
			_ = srv.Shutdown(shutdownCtx)
		}
		return exitOK
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			logger.Error("serve", "error", err)
			return exitNetwork
		}
		return exitOK
	}
}

func loadOrCreateToken(dataDir string) (string, bool, error) {
	if token := os.Getenv("HOSTEBIN_TOKEN"); token != "" {
		return token, false, nil
	}
	path := filepath.Join(dataDir, "token")
	if data, err := os.ReadFile(path); err == nil {
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", false, errors.New("token file is empty")
		}
		return token, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", false, err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", false, err
	}
	if _, err := io.WriteString(f, token+"\n"); err != nil {
		_ = f.Close()
		return "", false, err
	}
	if err := f.Close(); err != nil {
		return "", false, err
	}
	return token, true, nil
}

func parseBytes(raw string) (int64, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	multipliers := []struct {
		suffix string
		value  int64
	}{{"gib", 1 << 30}, {"mib", 1 << 20}, {"kib", 1 << 10}, {"gb", 1e9}, {"mb", 1e6}, {"kb", 1e3}, {"b", 1}}
	for _, unit := range multipliers {
		if before, ok := strings.CutSuffix(s, unit.suffix); ok {
			n, err := strconv.ParseInt(strings.TrimSpace(before), 10, 64)
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("invalid byte size %q", raw)
			}
			return n * unit.value, nil
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid byte size %q", raw)
	}
	return n, nil
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func envBool(key string) bool {
	value := strings.ToLower(os.Getenv(key))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
