<h1 align="center">hostebin</h1>

<p align="center">
  <em>Upload a bundle of files, get a URL you can hand to a person.</em>
</p>

<p align="center">
  <a href="https://github.com/readeem/hostebin/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/readeem/hostebin/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/readeem/hostebin/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/readeem/hostebin?sort=semver"></a>
  <a href="https://pkg.go.dev/github.com/readeem/hostebin"><img alt="Go reference" src="https://pkg.go.dev/badge/github.com/readeem/hostebin.svg"></a>
  <a href="LICENSE"><img alt="MIT license" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
</p>

---

Your agent just produced a 400-line HTML report. Pasting it into chat is useless.
`hostebin up report.html` returns a link that opens in a browser.

```console
$ hostebin up report.html
https://paste.example.com/b/9f2a1c7d40b8e35f/
```

One static Go binary is both the server and the client. Storage is a directory.
Reads are public, every mutation needs a bearer token, and the 128-bit bundle ID in
the URL is the read capability.

- **Bundles, not files.** Upload a page with its images and CSS; relative links work.
- **Markdown renders** to a styled page; `?raw=1` gives you the source.
- **Agent-shaped output.** `up` prints exactly one URL on stdout, diagnostics on
  stderr, `--json` for everything else. Ready-made [agent skills](skills/) included.
- **Serve it anywhere.** Localhost, a VPS with Let's Encrypt, or a tailnet with
  embedded Tailscale — no `tailscaled`, no `NET_ADMIN`.

## Install

<table>
<tr><th>Go</th><td>

```sh
go install github.com/readeem/hostebin/cmd/hostebin@latest
```

</td></tr>
<tr><th>Debian/Ubuntu</th><td>

```sh
curl -fsSLO https://github.com/readeem/hostebin/releases/latest/download/hostebin_linux_amd64.deb
sudo dpkg -i hostebin_linux_amd64.deb
```

</td></tr>
<tr><th>Fedora/RHEL,<br>Alpine</th><td>

```sh
sudo rpm -i https://github.com/readeem/hostebin/releases/latest/download/hostebin_linux_amd64.rpm
```

`.apk` packages are attached to every release too.

</td></tr>
<tr><th>Binary</th><td>

```sh
curl -fsSL https://github.com/readeem/hostebin/releases/latest/download/hostebin_linux_amd64.tar.gz \
  | tar xz hostebin && sudo install hostebin /usr/local/bin/
```

</td></tr>
<tr><th>Docker</th><td>

```sh
docker run --rm -p 8080:8080 -v hostebin-data:/data ghcr.io/readeem/hostebin
```

</td></tr>
</table>

> `go install`, the release binaries, the Linux packages, and the container image
> work from the first tagged release.

Archives, `checksums.txt`, and Linux packages are published for Linux, macOS,
Windows, and FreeBSD on amd64, arm64, 386, and armv7. File names carry no version,
so `releases/latest/download/...` always resolves to the newest release. Verify a
download with `sha256sum -c checksums.txt --ignore-missing`.

## Quick start

**1. Start a server** (any machine that both you and your agent can reach):

```sh
hostebin serve
```

On first run it generates a token, stores it in the data directory with mode `0600`,
and logs it once.

**2. Point the client at it:**

```sh
export HOSTEBIN_SERVER=http://localhost:8080
export HOSTEBIN_TOKEN="$(cat "${XDG_DATA_HOME:-$HOME/.local/share}/hostebin/token")"
```

**3. Upload:**

```sh
hostebin up report.html                       # one page
hostebin up report.html img/chart.png         # page plus assets
hostebin up ./site/                           # a whole directory
echo '# Status' | hostebin up -n status.md -  # from stdin
```

To persist the settings instead of exporting them every session, put them in
`~/.config/hostebin/config.json` (see [Configuration](#configuration)).

## Using it from an agent

The [`skills/`](skills/) directory contains a drop-in skill that teaches an agent
when and how to publish. For Claude Code:

```sh
mkdir -p ~/.claude/skills && cp -r skills/hostebin ~/.claude/skills/
```

For any other agent, `skills/hostebin/SKILL.md` is plain Markdown — append it to
`AGENTS.md` or your system prompt.

The contract that makes this safe to script:

| Behaviour | Detail |
| --- | --- |
| stdout | Exactly one line: the bundle URL |
| stderr | Every diagnostic, warning, and error |
| `--json` | Full response: `id`, `url`, `entry_url`, `files[]`, `expires_at` |
| exit codes | `0` success, `1` usage/config error, `2` network or server error |
| config | `HOSTEBIN_SERVER` and `HOSTEBIN_TOKEN` are all an agent needs |

```sh
url=$(hostebin up --ttl 7d --title 'Weekly report' report.html) || exit $?
echo "Report: $url"
```

Iterate on an artifact without invalidating the link you already shared:

```sh
hostebin up --id 9f2a1c7d40b8e35f report.html
```

## Commands

```text
hostebin up [flags] <file|directory|->...   upload a bundle, print its URL
hostebin ls [flags]                         list live bundles
hostebin rm [flags] <id>                    delete a bundle
hostebin serve [flags]                      run the server
hostebin version                            version, commit, build date
```

`up` flags:

| Flag | Meaning |
| --- | --- |
| `--title TEXT` | Bundle title |
| `--ttl DURATION` | `30m`, `12h`, `7d`, `2w`, or `never` |
| `--entry PATH` | File served at the bundle root |
| `--id ID` | Replace an existing bundle in place |
| `-n, --name PATH` | Name for data read from stdin (required with `-`) |
| `--json` | Print the JSON response instead of the URL |
| `--open` | Open the URL in the system browser |
| `--quiet` | Suppress optional diagnostics |
| `--server`, `--token`, `--config` | Override the resolved configuration |

Directory uploads strip the directory argument itself and keep the paths below it;
symlinks and non-regular files are refused. `--entry` picks the root page; otherwise
a single file wins, then `index.html`, then the first HTML file, then the first
Markdown file, then a generated listing.

## Configuration

Precedence: **flags → `HOSTEBIN_*` environment → JSON config → defaults.** Every long
flag has an environment variable formed by uppercasing it and replacing `-` with `_`
(`--max-upload` → `HOSTEBIN_MAX_UPLOAD`).

A `0600` config file is created on first use at
`${XDG_CONFIG_HOME:-~/.config}/hostebin/config.json` (Linux/BSD),
`~/Library/Application Support/hostebin/config.json` (macOS), or `%AppData%\hostebin`
(Windows). Override with `--config` or `HOSTEBIN_CONFIG`. JSON keys are the long flag
names:

```json
{
  "server": "https://hostebin.example.com",
  "token": "replace-me",
  "host": "",
  "port": 8080,
  "data": "/home/me/.local/share/hostebin",
  "base-url": "",
  "tls-addr": ":8443",
  "tls-cert": "",
  "tls-key": "",
  "acme-domain": "",
  "acme-email": "",
  "tailscale": false,
  "funnel": false,
  "ts-hostname": "hostebin",
  "ts-auth-key": "",
  "max-upload": "32MiB",
  "max-files": 64,
  "default-ttl": "never",
  "csp": "",
  "bundle-host": ""
}
```

Persistent data (bundles plus the `token` file) lives in
`${XDG_DATA_HOME:-~/.local/share}/hostebin` on Linux/BSD and `<user config
dir>/hostebin/data` elsewhere.

## Running a server

```sh
# plain HTTP, the default
hostebin serve --host 0.0.0.0 --port 8080 --data /var/lib/hostebin

# your own certificate, with or without plain HTTP alongside
hostebin serve --port 0 --tls-addr :8443 --tls-cert cert.pem --tls-key key.pem

# automatic public HTTPS; owns :80 (ACME challenges + redirect) and :443
hostebin serve --port 0 --acme-domain files.example.com --acme-email ops@example.com

# embedded Tailscale — no tailscaled, no /dev/net/tun, no NET_ADMIN
HOSTEBIN_TS_AUTH_KEY=tskey-auth-... hostebin serve --port 0 --tailscale
HOSTEBIN_TS_AUTH_KEY=tskey-auth-... hostebin serve --port 0 --funnel   # public
```

Tailscale state persists under `$HOSTEBIN_DATA/tsnet`; the server tries tailnet HTTPS
first and logs an HTTP fallback if HTTPS is not enabled for the tailnet.

`--base-url` overrides the URL returned by every listener. Without it, Tailscale and
ACME use their known DNS names and other listeners derive links from the request. Behind
a reverse proxy, forward the original `Host` and set `X-Forwarded-Proto`.

Useful server settings: `--max-upload` (default `32MiB`), `--max-files` (default `64`),
`--default-ttl` (default `never`), `--csp` (`off` disables it). Expired bundles 404
immediately; a sweep at startup and every ten minutes reclaims their files.

### Per-bundle origins

`--bundle-host '*.paste.example.com'` serves each bundle from its own subdomain —
`<id>.paste.example.com` — instead of `/b/<id>/`. Every bundle then gets a distinct
browser origin, so one cannot read another's `localStorage` or fetch its files, and
the API is cross-origin to all of them. Existing `/b/<id>/…` links 301 to the new
origin, and upload responses return the subdomain form, so no client changes.

Requires a **wildcard certificate**, which the built-in ACME support cannot issue:
`autocert` implements only `http-01` and `tls-alpn-01`, while Let's Encrypt issues
wildcards only over `dns-01`. Combining `--bundle-host` with `--acme-domain` is a
startup error rather than a silent fallback — per-host issuance would publish every
bundle id to the public Certificate Transparency logs, defeating the point of an
unguessable link. Terminate TLS at a reverse proxy holding `*.paste.example.com`
(forwarding the original `Host` and `X-Forwarded-Proto`), or pass the certificate
with `--tls-cert`/`--tls-key`.

Note that the id moves from the URL path into the hostname, so it becomes visible
to DNS resolvers and to anyone on the network path via TLS SNI. Bundle responses
send `Referrer-Policy: no-referrer` so the id is not handed to third-party origins
the page loads from.

The `.deb`, `.rpm`, and Arch packages install a hardened systemd unit:

```sh
sudo systemctl enable --now hostebin
sudo systemctl cat hostebin      # data lives in /var/lib/hostebin
```

### Containers

```sh
docker run --rm -p 8080:8080 -v hostebin-data:/data ghcr.io/readeem/hostebin

HOSTEBIN_TOKEN=choose-a-secret docker compose --profile plain up
HOSTEBIN_TS_AUTH_KEY=tskey-auth-... HOSTEBIN_TOKEN=choose-a-secret \
  docker compose --profile tailscale up
```

Images are `CGO_ENABLED=0` on distroless nonroot, published for `linux/amd64` and
`linux/arm64`. The Tailscale Compose service needs no elevated capabilities.

## HTTP API

All `/api/v1` routes require `Authorization: Bearer TOKEN`. Bundle content and
`/healthz` are public.

```sh
curl -sf -H "Authorization: Bearer $HOSTEBIN_TOKEN" \
  -H 'X-Hostebin-Filename: report.html' \
  --data-binary @report.html \
  "$HOSTEBIN_SERVER/api/v1/bundles"

curl -sf -H "Authorization: Bearer $HOSTEBIN_TOKEN" \
  -F 'file=@report.html;filename=report.html' \
  -F 'file=@img/chart.png;filename=img/chart.png' \
  "$HOSTEBIN_SERVER/api/v1/bundles"
```

| Route | Purpose |
| --- | --- |
| `POST /api/v1/bundles` | Create from multipart `file` parts or a raw body with `X-Hostebin-Filename` |
| `PUT /api/v1/bundles/{id}?mode=replace` | Atomically replace a bundle |
| `PUT /api/v1/bundles/{id}?mode=merge` | Atomically merge named files into a bundle |
| `GET /api/v1/bundles` | List live bundles |
| `DELETE /api/v1/bundles/{id}` | Delete a bundle |
| `GET /b/{id}/...` | Read public content; `?raw=1` disables Markdown rendering |

Raw uploads also accept `X-Hostebin-Title`, `X-Hostebin-Entry`, and `X-Hostebin-TTL`;
multipart uploads use `title`, `entry`, and `ttl` fields.

## Content security model

Hosted files are untrusted. The server never uses cookie authentication, sends
`X-Content-Type-Options: nosniff`, and serves unknown extensions as downloadable
`application/octet-stream`. Bundle responses carry this default CSP:

```text
default-src 'self' data: blob: https: 'unsafe-inline' 'unsafe-eval'; connect-src 'self' https:; form-action 'none'; frame-ancestors 'none'
```

Inline scripts/styles and HTTPS CDN assets are intentionally allowed because generated
reports need them, and `connect-src` permits HTTPS so a report can render live data.
Plain HTTP is deliberately excluded: it keeps a published page from probing the
reader's own network (`http://192.168.x.x`, `http://localhost:11434`) from their
browser.

This is not a sandbox. `script-src` falls back to `https:`, so a bundle can already
load and execute code from any HTTPS origin, and `img-src` can carry data outward in a
URL — the CSP limits accidents, not a malicious uploader. The threat model is that
whoever holds the upload token is trusted, and the reader and their network are not.

By default bundles share an origin, so content can reach another bundle whose
unguessable ID it knows. `--bundle-host` gives each bundle its own origin and removes
that class entirely; see [Per-bundle origins](#per-bundle-origins).

**While bundles share an origin with the API, the API must never accept ambient
credentials.** It is bearer-token only for exactly this reason: a cookie session would
let any published page call `/api/v1/*` with the reader's credentials.

Writes stage in hidden temporary directories before an atomic rename; reads go through
`os.OpenRoot`, so `..` and symlinks cannot escape the storage root.

**Treat every bundle URL as a secret-bearing link, and never publish credentials or
personal data.**

## Development

[`just`](https://github.com/casey/just) drives everything:

```sh
just              # list recipes
just build        # ./hostebin, version stamped from the current git tag
just check        # gofmt, go vet, go test — the CI gate
just test-all     # default, notsnet, and -race test runs
just serve        # run the server from source
```

Version lives in exactly one place, [`internal/version`](internal/version/version.go),
and every build path (justfile, Dockerfile, GoReleaser) overrides it with the same
`-ldflags` target. A build without ldflags falls back to the module version recorded
by the Go toolchain.

Smaller binary that rejects Tailscale flags:

```sh
just build-slim   # go build -tags notsnet
```

## Releasing

```sh
just tag 0.1.0    # tags v0.1.0 and pushes it
```

The tag triggers [`release.yml`](.github/workflows/release.yml): GoReleaser builds
every target, attaches archives, checksums, and Linux packages to the GitHub release,
updates the Homebrew/Scoop/AUR manifests, and a second job publishes the container
image to GHCR.

## License

[MIT](LICENSE) © Readeem
