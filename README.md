# hostebin

`hostebin` uploads a small bundle of files and gives you a web URL. It is designed for agents that need to hand a rendered HTML page, Markdown report, chart, or related assets directly to a person.

The same static Go binary is the server and the client. Storage is a directory, reads are public, and every API mutation requires a bearer token. A 128-bit random bundle ID is the read capability.

## Build and start

```sh
go build -o hostebin ./cmd/hostebin
./hostebin serve --addr :8080 --data ./data
```

On first start, the server creates `./data/token` with mode `0600` and logs the generated token once. In another shell:

```sh
export HOSTEBIN_URL=http://localhost:8080
export HOSTEBIN_TOKEN="$(cat ./data/token)"

URL=$(./hostebin up plan.html)
printf '%s\n' "$URL"
```

`hostebin up` writes exactly one URL plus a newline to stdout. Diagnostics go to stderr. Use `--json` when another program needs the complete response.

## Uploads

Files named together form one virtual folder, so relative links continue to work:

```sh
hostebin up plan.html img/diagram.png styles/report.css
hostebin up ./report/
echo '# Status' | hostebin up -n status.md -
hostebin up --title 'Weekly report' --ttl 7d report.html
hostebin up --id 01abc... report.html
```

Directory uploads strip the directory argument itself and retain paths below it. Symlinks and non-regular files are refused. `--entry` selects the page shown at the bundle root. Otherwise a single file is the entry; multiple files prefer `index.html`, then the first HTML file, then the first Markdown file, and finally a generated listing.

The `up` flags are:

```text
--title TEXT       bundle title
--ttl DURATION     expiry such as 30m, 7d, or never
--entry PATH       root entry file
--json             emit the JSON response
--id ID            replace an existing bundle in place
--open             open the URL in the system browser
--server URL       override HOSTEBIN_URL/config
--token TOKEN      override HOSTEBIN_TOKEN/config
--quiet            suppress optional diagnostics
-n, --name PATH    name used for stdin
```

List and remove bundles with `hostebin ls` and `hostebin rm ID`. Both accept `--server` and `--token`; `ls --json` returns unmodified structured metadata.

Client configuration precedence is flags, then `HOSTEBIN_URL`/`HOSTEBIN_TOKEN`, then `$XDG_CONFIG_HOME/hostebin/config.json`:

```json
{"url":"https://hostebin.example.com","token":"replace-me"}
```

## HTTP API

All `/api/v1` routes require `Authorization: Bearer TOKEN`. Bundle content and `/healthz` are public.

```sh
curl -sf -H "Authorization: Bearer $HOSTEBIN_TOKEN" \
  -H 'X-Hostebin-Filename: plan.html' \
  --data-binary @plan.html \
  "$HOSTEBIN_URL/api/v1/bundles"

curl -sf -H "Authorization: Bearer $HOSTEBIN_TOKEN" \
  -F 'file=@plan.html;filename=plan.html' \
  -F 'file=@img/diagram.png;filename=img/diagram.png' \
  "$HOSTEBIN_URL/api/v1/bundles"
```

Routes:

| Route | Purpose |
| --- | --- |
| `POST /api/v1/bundles` | Create from multipart `file` parts or a raw body with `X-Hostebin-Filename` |
| `PUT /api/v1/bundles/{id}?mode=replace` | Atomically replace a bundle |
| `PUT /api/v1/bundles/{id}?mode=merge` | Atomically merge named files into a bundle |
| `GET /api/v1/bundles` | List live bundles |
| `DELETE /api/v1/bundles/{id}` | Delete a bundle |
| `GET /b/{id}/...` | Read public content; `?raw=1` disables Markdown rendering |

Raw requests can also set `X-Hostebin-Title`, `X-Hostebin-Entry`, and `X-Hostebin-TTL`. Multipart requests use fields named `title`, `entry`, and `ttl`.

## Server configuration

Plain HTTP is enabled by default on `:8080`:

```sh
hostebin serve --addr :8080 --data /var/lib/hostebin
```

Use an existing certificate independently or alongside HTTP:

```sh
hostebin serve --addr= --tls-addr :8443 --tls-cert cert.pem --tls-key key.pem
```

Automatic public HTTPS uses Let's Encrypt and owns ports 80 and 443. Port 80 serves ACME challenges and redirects other requests:

```sh
hostebin serve --addr= --acme-domain files.example.com --acme-email ops@example.com
```

Embedded Tailscale needs neither `tailscaled`, `/dev/net/tun`, nor `NET_ADMIN`:

```sh
TS_AUTHKEY=tskey-auth-... hostebin serve --addr= --tailscale
TS_AUTHKEY=tskey-auth-... hostebin serve --addr= --funnel
```

The server tries tailnet HTTPS first and logs an HTTP fallback when HTTPS is not enabled for the tailnet. State is persistent under `$HOSTEBIN_DATA/tsnet`. `--funnel` exposes the HTTPS listener publicly according to the tailnet policy.

`--base-url` overrides the URL returned by every listener. Without it, Tailscale and ACME use their known DNS identities; other listeners derive links from the request host and protocol. Behind a reverse proxy, forward the original `Host` and set `X-Forwarded-Proto`.

Relevant environment variables:

| Variable | Default |
| --- | --- |
| `HOSTEBIN_DATA` | `./data` |
| `HOSTEBIN_TOKEN` | token file, generated when absent |
| `HOSTEBIN_MAX_UPLOAD` | `32MiB` |
| `HOSTEBIN_MAX_FILES` | `64` |
| `HOSTEBIN_DEFAULT_TTL` | no expiry |
| `HOSTEBIN_CSP` | built-in policy; `off` disables it |
| `HOSTEBIN_TS` | disabled |
| `HOSTEBIN_TS_HOSTNAME` | `hostebin` |
| `TS_AUTHKEY` | unset |

Expired bundles return 404 immediately. A sweep runs at startup and every ten minutes to reclaim their files.

## Containers

```sh
docker build -t hostebin .
docker run --rm -p 8080:8080 -v hostebin-data:/data hostebin

HOSTEBIN_TOKEN=choose-a-secret docker compose --profile plain up --build
TS_AUTHKEY=tskey-auth-... HOSTEBIN_TOKEN=choose-a-secret docker compose --profile tailscale up --build
```

The image is built with `CGO_ENABLED=0` and runs as the distroless non-root user. The Tailscale Compose service has no elevated capabilities or host tunnel device.

For a smaller build that explicitly rejects Tailscale flags:

```sh
CGO_ENABLED=0 go build -tags notsnet -o hostebin ./cmd/hostebin
```

## Content security model

Hosted files are untrusted. The server never uses cookie authentication, adds `X-Content-Type-Options: nosniff`, and serves unknown extensions as downloadable `application/octet-stream`. It applies this default CSP to bundle responses:

```text
default-src 'self' data: blob: https: 'unsafe-inline' 'unsafe-eval'; connect-src 'self'; form-action 'none'; frame-ancestors 'none'
```

This intentionally permits inline scripts/styles and HTTPS CDN assets because generated reports commonly need them. It is not full isolation: every bundle shares an origin, so malicious content can interact with another bundle when it knows that bundle's unguessable ID. True isolation requires per-bundle origins and is outside v1.

On-disk writes stage in hidden temporary directories before rename. Reads use Go's `os.OpenRoot`, preventing `..` and symlink escapes at the filesystem boundary.

## Test

```sh
go test ./...
go test -tags notsnet ./...
```
