package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/readeem/hostebin/internal/listen"
	"github.com/readeem/hostebin/internal/logging"
	"github.com/readeem/hostebin/internal/server"
	"github.com/readeem/hostebin/internal/store"
	"github.com/readeem/hostebin/internal/users"
	"github.com/readeem/hostebin/internal/users/filestore"
	"github.com/rs/zerolog"
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
	logger := logging.NewConsole(stderr)
	httpAddr, err := httpListenAddr(cfg.HTTPHost, cfg.HTTPPort)
	if err != nil {
		logger.Error().Err(err).Msg("invalid HTTP listener")
		return exitUsage
	}
	if httpAddr == "" && cfg.TLSCert == "" && cfg.TLSKey == "" && cfg.ACMEDomain == "" && !cfg.Tailscale && !cfg.Funnel {
		logger.Error().Msg("HTTP listener is disabled because port is 0, and no TLS, ACME, or Tailscale listener is enabled; set --port to a value between 1 and 65535 or enable another listener")
		return exitUsage
	}
	st, err := store.New(cfg.Data)
	if err != nil {
		logger.Error().Err(err).Msg("initialize storage")
		return exitNetwork
	}
	bootstrapToken, err := loadBootstrapToken(st.DataDir(), cfg.Token)
	if err != nil {
		logger.Error().Err(err).Msg("initialize token")
		return exitNetwork
	}
	userStore, err := filestore.Open(st.DataDir())
	if err != nil {
		logger.Error().Err(err).Msg("initialize users")
		return exitNetwork
	}
	defer userStore.Close()
	userService := users.NewService(userStore)
	// Bootstrap mints the admin token itself when there is nothing to adopt, so
	// generation lives in exactly one place; here we only persist and announce it.
	adminID, generatedToken, err := userService.Bootstrap(context.Background(), bootstrapToken)
	if err != nil {
		logger.Error().Err(err).Msg("bootstrap admin user")
		return exitNetwork
	}
	if generatedToken != "" {
		if err := writeBootstrapToken(st.DataDir(), generatedToken); err != nil {
			logger.Error().Err(err).Msg("persist generated token")
			return exitNetwork
		}
		logger.Info().Str("token", generatedToken).Str("user", "admin").Str("path", filepath.Join(st.DataDir(), "token")).Msg("generated upload token")
	}
	adopted, err := st.AdoptUnownedBundles(adminID)
	if err != nil {
		logger.Error().Err(err).Msg("adopt legacy bundles")
		return exitNetwork
	}
	if adopted > 0 {
		logger.Info().Int("count", adopted).Str("user", "admin").Msg("adopted legacy bundles")
	}
	maxUpload, err := parseBytes(cfg.MaxUpload)
	if err != nil {
		logger.Error().Err(err).Msg("invalid max-upload")
		return exitUsage
	}
	if cfg.MaxFiles <= 0 {
		logger.Error().Msg("max-files must be positive")
		return exitUsage
	}
	var defaultTTL time.Duration
	if raw := cfg.DefaultTTL; raw != "" && !strings.EqualFold(raw, "never") {
		defaultTTL, err = server.ParseDuration(raw)
		if err != nil || defaultTTL <= 0 {
			logger.Error().Msg("invalid default-ttl")
			return exitUsage
		}
	}
	csp := cfg.CSP
	if csp == "" {
		csp = server.DefaultCSP
	}
	if cfg.BundleHost != "" && cfg.ACMEDomain != "" {
		logger.Error().Msg("--bundle-host cannot be combined with --acme-domain: built-in ACME cannot issue a wildcard certificate, and would publish every bundle id to Certificate Transparency logs. Terminate TLS at a reverse proxy holding a wildcard certificate, or supply one with --tls-cert/--tls-key")
		return exitUsage
	}
	app, err := server.New(
		server.Config{
			Store:      st,
			Users:      userService,
			MaxUpload:  maxUpload,
			MaxFiles:   cfg.MaxFiles,
			DefaultTTL: defaultTTL,
			CSP:        csp,
			BundleHost: cfg.BundleHost,
			Logger:     logger,
		},
	)
	if err != nil {
		logger.Error().Err(err).Msg("initialize server")
		return exitNetwork
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	listeners, err := listen.Build(
		ctx,
		listen.Config{
			Addr:       httpAddr,
			BaseURL:    cfg.BaseURL,
			TLSAddr:    cfg.TLSAddr,
			TLSCert:    cfg.TLSCert,
			TLSKey:     cfg.TLSKey,
			ACMEDomain: cfg.ACMEDomain,
			ACMEEmail:  cfg.ACMEEmail,
			DataDir:    st.DataDir(),
			Tailscale:  cfg.Tailscale || cfg.Funnel,
			Funnel:     cfg.Funnel,
			TSHostname: cfg.TSHostname,
			TSAuthKey:  cfg.TSAuthKey,
			Logf:       func(format string, values ...any) { logger.Info().Msgf(format, values...) },
		},
	)
	if err != nil {
		logger.Error().Err(err).Msg("initialize listeners")
		return exitNetwork
	}
	defer listeners.Close()
	gcStop := make(chan struct{})
	defer close(gcStop)

	go st.RunGC(gcStop, 10*time.Minute, func(n int, err error) {
		if err != nil {
			logger.Error().Err(err).Msg("garbage collection")
		} else if n > 0 {
			logger.Info().Int("count", n).Msg("removed expired bundles")
		}
	})
	errCh := make(chan error, len(listeners.Endpoints))
	servers := make([]*http.Server, 0, len(listeners.Endpoints))

	for i, endpoint := range listeners.Endpoints {
		handler := endpoint.Handler
		if handler == nil {
			handler = app.Handler()
			if endpoint.BaseURL != "" {
				handler = server.WithBaseURL(endpoint.BaseURL, handler)
			}
		}
		httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
		servers = append(servers, httpServer)
		logListening(logger, endpoint, cfg, st.DataDir(), maxUpload, defaultTTL, csp, i == 0)

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
			logger.Error().Err(err).Msg("serve")
			return exitNetwork
		}
		return exitOK
	}
}

func logListening(logger *zerolog.Logger, endpoint listen.Endpoint, cfg *Config, dataDir string, maxUpload int64, defaultTTL time.Duration, csp string, includeConfig bool) {
	event := logger.Info().Str("address", endpoint.Listener.Addr().String())
	if endpoint.BaseURL != "" {
		event = event.Str("bundle_base_url", endpoint.BaseURL)
	}
	if !includeConfig {
		event.Msg("listening")
		return
	}
	ttl := "never"
	if defaultTTL > 0 {
		ttl = defaultTTL.String()
	}
	cspMode := "custom"
	if strings.EqualFold(csp, "off") {
		cspMode = "disabled"
	} else if cfg.CSP == "" {
		cspMode = "default"
	}
	event = event.Int64("max_upload_bytes", maxUpload).
		Int("max_files", cfg.MaxFiles).
		Str("default_ttl", ttl).
		Str("csp", cspMode).
		Bool("certificate_tls", cfg.TLSCert != "").
		Bool("tailscale", cfg.Tailscale || cfg.Funnel).
		Bool("funnel", cfg.Funnel)
	if cfg.ConfigFile != "" {
		event = event.Str("config_file", cfg.ConfigFile)
	}
	if dataDir != "" {
		event = event.Str("data_dir", dataDir)
	}
	if cfg.ACMEDomain != "" {
		event = event.Str("acme_domain", cfg.ACMEDomain)
	}
	if cfg.BundleHost != "" {
		event = event.Str("bundle_host", cfg.BundleHost)
	}
	event.Msg("listening")
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

func loadBootstrapToken(dataDir, configuredToken string) (string, error) {
	if configuredToken != "" {
		return configuredToken, nil
	}
	path := filepath.Join(dataDir, "token")
	if data, err := os.ReadFile(path); err == nil {
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", errors.New("token file is empty")
		}
		return token, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return "", nil
}

func writeBootstrapToken(dataDir, token string) error {
	path := filepath.Join(dataDir, "token")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(f, token+"\n"); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
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
