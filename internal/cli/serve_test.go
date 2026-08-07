package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/readeem/hostebin/internal/listen"
	"github.com/readeem/hostebin/internal/server"
	"github.com/rs/zerolog"
)

func TestLogListeningIncludesAppliedConfiguration(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var output bytes.Buffer
	logger := zerolog.New(&output)
	cfg := &Config{
		ConfigFile: "/etc/hostebin/config.json",
		BaseURL:    "https://bundles.example/",
		TLSCert:    "/etc/hostebin/tls.crt",
		ACMEDomain: "acme.example",
		Tailscale:  true,
		Funnel:     true,
		MaxFiles:   12,
		Token:      "secret-token",
		TSAuthKey:  "secret-auth-key",
	}

	logListening(&logger, listen.Endpoint{Listener: listener, BaseURL: "https://bundles.example"}, cfg, "/var/lib/hostebin", 4<<20, 2*time.Hour, server.DefaultCSP, true)

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	wants := map[string]any{
		"message":          "listening",
		"address":          listener.Addr().String(),
		"config_file":      "/etc/hostebin/config.json",
		"data_dir":         "/var/lib/hostebin",
		"max_upload_bytes": float64(4 << 20),
		"max_files":        float64(12),
		"default_ttl":      "2h0m0s",
		"csp":              "default",
		"certificate_tls":  true,
		"tailscale":        true,
		"funnel":           true,
		"bundle_base_url":  "https://bundles.example",
		"acme_domain":      "acme.example",
	}
	for key, want := range wants {
		if got := event[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
	if _, ok := event["token"]; ok {
		t.Error("configuration log contains token")
	}
	if _, ok := event["ts_auth_key"]; ok {
		t.Error("configuration log contains Tailscale auth key")
	}
}

func TestLogListeningOmitsUnsetStrings(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var output bytes.Buffer
	logger := zerolog.New(&output)
	cfg := &Config{MaxFiles: 64}

	logListening(&logger, listen.Endpoint{Listener: listener}, cfg, "/data", 32<<20, 0, "off", true)

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event["default_ttl"] != "never" || event["csp"] != "disabled" {
		t.Fatalf("defaults logged as ttl=%#v csp=%#v", event["default_ttl"], event["csp"])
	}
	if _, ok := event["bundle_base_url"]; ok {
		t.Error("empty bundle base URL was logged")
	}
	if _, ok := event["config_file"]; ok {
		t.Error("empty config file was logged")
	}
}

func TestLogListeningDoesNotRepeatConfiguration(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var output bytes.Buffer
	logger := zerolog.New(&output)

	logListening(&logger, listen.Endpoint{Listener: listener, BaseURL: "https://bundles.example"}, nil, "", 0, 0, "", false)

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event["address"] != listener.Addr().String() {
		t.Errorf("address = %#v, want %q", event["address"], listener.Addr())
	}
	if event["bundle_base_url"] != "https://bundles.example" {
		t.Errorf("bundle_base_url = %#v", event["bundle_base_url"])
	}
	if _, ok := event["config_file"]; ok {
		t.Error("configuration repeated on a subsequent listener")
	}
}
