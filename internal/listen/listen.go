package listen

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/acme/autocert"
)

type Config struct {
	Addr       string
	BaseURL    string
	TLSAddr    string
	TLSCert    string
	TLSKey     string
	ACMEDomain string
	ACMEEmail  string
	DataDir    string
	Tailscale  bool
	Funnel     bool
	TSHostname string
	TSAuthKey  string
	Logf       func(string, ...any)
}

type Endpoint struct {
	Listener net.Listener
	BaseURL  string
	Handler  http.Handler
}

type Set struct {
	Endpoints []Endpoint
	closers   []interface{ Close() error }
}

func (s *Set) Close() error {
	var errs []error
	for _, closer := range s.closers {
		if err := closer.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func Build(ctx context.Context, cfg Config) (*Set, error) {
	set := &Set{}
	fail := func(err error) (*Set, error) { _ = set.Close(); return nil, err }
	if cfg.Addr != "" {
		ln, err := net.Listen("tcp", cfg.Addr)
		if err != nil {
			return fail(fmt.Errorf("listen on %s: %w", cfg.Addr, err))
		}
		set.Endpoints = append(set.Endpoints, Endpoint{Listener: ln, BaseURL: cfg.BaseURL})
		set.closers = append(set.closers, ln)
	}
	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		if cfg.TLSCert == "" || cfg.TLSKey == "" {
			return fail(errors.New("--tls-cert and --tls-key must be used together"))
		}
		addr := cfg.TLSAddr
		if addr == "" {
			addr = ":8443"
		}
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return fail(fmt.Errorf("load TLS certificate: %w", err))
		}
		tcp, err := net.Listen("tcp", addr)
		if err != nil {
			return fail(fmt.Errorf("TLS listen on %s: %w", addr, err))
		}
		ln := tls.NewListener(tcp, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		set.Endpoints = append(set.Endpoints, Endpoint{Listener: ln, BaseURL: cfg.BaseURL})
		set.closers = append(set.closers, ln)
	}
	if cfg.ACMEDomain != "" {
		if cfg.DataDir == "" {
			return fail(errors.New("data directory is required for ACME"))
		}
		cache := filepath.Join(cfg.DataDir, "acme")
		if err := os.MkdirAll(cache, 0o700); err != nil {
			return fail(err)
		}
		manager := &autocert.Manager{Prompt: autocert.AcceptTOS, Email: cfg.ACMEEmail, Cache: autocert.DirCache(cache), HostPolicy: autocert.HostWhitelist(cfg.ACMEDomain)}
		httpLn, err := net.Listen("tcp", ":80")
		if err != nil {
			return fail(fmt.Errorf("ACME HTTP listen: %w", err))
		}
		httpsTCP, err := net.Listen("tcp", ":443")
		if err != nil {
			_ = httpLn.Close()
			return fail(fmt.Errorf("ACME HTTPS listen: %w", err))
		}
		base := "https://" + cfg.ACMEDomain
		if cfg.BaseURL != "" {
			base = strings.TrimRight(cfg.BaseURL, "/")
		}
		redirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + cfg.ACMEDomain + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})
		set.Endpoints = append(set.Endpoints,
			Endpoint{Listener: httpLn, Handler: manager.HTTPHandler(redirect)},
			Endpoint{Listener: tls.NewListener(httpsTCP, manager.TLSConfig()), BaseURL: base},
		)
		set.closers = append(set.closers, httpLn, httpsTCP)
	}
	if cfg.Tailscale || cfg.Funnel {
		if err := addTailscale(ctx, cfg, set); err != nil {
			return fail(err)
		}
	}
	if len(set.Endpoints) == 0 {
		return fail(errors.New("no listeners enabled"))
	}
	return set, nil
}
