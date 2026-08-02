//go:build !notsnet

package listen

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"tailscale.com/tsnet"
)

func addTailscale(ctx context.Context, cfg Config, set *Set) error {
	hostname := cfg.TSHostname
	if hostname == "" {
		hostname = "hostebin"
	}
	ts := &tsnet.Server{Dir: filepath.Join(cfg.DataDir, "tsnet"), Hostname: hostname, AuthKey: cfg.TSAuthKey}
	if cfg.Logf != nil {
		ts.UserLogf = func(format string, args ...any) { cfg.Logf(format, args...) }
	}
	status, err := ts.Up(ctx)
	if err != nil {
		_ = ts.Close()
		return fmt.Errorf("start tsnet: %w", err)
	}
	dnsName := strings.TrimSuffix(status.Self.DNSName, ".")
	var ln net.Listener
	base := "https://" + dnsName
	if cfg.Funnel {
		ln, err = ts.ListenFunnel("tcp", ":443")
		if err != nil {
			_ = ts.Close()
			return fmt.Errorf("start Tailscale Funnel: %w", err)
		}
	} else {
		ln, err = ts.ListenTLS("tcp", ":443")
		if err != nil {
			if cfg.Logf != nil {
				cfg.Logf("Tailscale HTTPS unavailable (%v); falling back to HTTP", err)
			}
			ln, err = ts.Listen("tcp", ":80")
			base = "http://" + dnsName
		}
		if err != nil {
			_ = ts.Close()
			return fmt.Errorf("start tsnet listener: %w", err)
		}
	}
	if cfg.BaseURL != "" {
		base = strings.TrimRight(cfg.BaseURL, "/")
	}
	set.Endpoints = append(set.Endpoints, Endpoint{Listener: ln, BaseURL: base})
	set.closers = append(set.closers, ts)
	return nil
}
