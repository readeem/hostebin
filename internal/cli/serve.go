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
	cfg, err := NewConfig()
	if err != nil {
		fmt.Fprintln(stderr, "hostebin:", err)
		return exitUsage
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg.registerServeFlags(fs)
	if err := parseConfig(fs, args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: hostebin serve [flags]")
		return exitUsage
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	httpAddr, err := httpListenAddr(cfg.HTTPHost, cfg.HTTPPort)
	if err != nil {
		logger.Error("invalid HTTP listener", "error", err)
		return exitUsage
	}
	st, err := store.New(cfg.Data)
	if err != nil {
		logger.Error("initialize storage", "error", err)
		return exitNetwork
	}
	token, generated, err := loadOrCreateToken(st.DataDir(), cfg.Token)
	if err != nil {
		logger.Error("initialize token", "error", err)
		return exitNetwork
	}
	if generated {
		logger.Info("generated upload token", "token", token, "path", filepath.Join(st.DataDir(), "token"))
	}
	maxUpload, err := parseBytes(cfg.MaxUpload)
	if err != nil {
		logger.Error("invalid max-upload", "error", err)
		return exitUsage
	}
	if cfg.MaxFiles <= 0 {
		logger.Error("max-files must be positive")
		return exitUsage
	}
	var defaultTTL time.Duration
	if raw := cfg.DefaultTTL; raw != "" && !strings.EqualFold(raw, "never") {
		defaultTTL, err = server.ParseDuration(raw)
		if err != nil || defaultTTL <= 0 {
			logger.Error("invalid default-ttl")
			return exitUsage
		}
	}
	csp := cfg.CSP
	if csp == "" {
		csp = server.DefaultCSP
	}
	app, err := server.New(server.Config{Store: st, Token: token, MaxUpload: maxUpload, MaxFiles: cfg.MaxFiles, DefaultTTL: defaultTTL, CSP: csp, Logger: logger})
	if err != nil {
		logger.Error("initialize server", "error", err)
		return exitNetwork
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	listeners, err := listen.Build(ctx, listen.Config{Addr: httpAddr, BaseURL: cfg.BaseURL, TLSAddr: cfg.TLSAddr, TLSCert: cfg.TLSCert, TLSKey: cfg.TLSKey, ACMEDomain: cfg.ACMEDomain, ACMEEmail: cfg.ACMEEmail, DataDir: st.DataDir(), Tailscale: cfg.Tailscale || cfg.Funnel, Funnel: cfg.Funnel, TSHostname: cfg.TSHostname, TSAuthKey: cfg.TSAuthKey, Logf: func(format string, values ...any) { logger.Info(fmt.Sprintf(format, values...)) }})
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

func httpListenAddr(host string, port int) (string, error) {
	if port < 0 || port > 65535 {
		return "", fmt.Errorf("port must be between 0 and 65535")
	}
	if port == 0 {
		return "", nil
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func loadOrCreateToken(dataDir, configuredToken string) (string, bool, error) {
	if configuredToken != "" {
		return configuredToken, false, nil
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
