# hostebin reference

Complete surface of the CLI and HTTP API. Load this when the basic `up`/`ls`/`rm`
flow in `SKILL.md` is not enough — scripting against the API, running a server, or
debugging configuration.

## Commands

```text
hostebin up [flags] <file|directory|->...   upload; prints one URL
hostebin ls [flags]                         list live bundles
hostebin rm [flags] <id>                    delete a bundle
hostebin serve [flags]                      run the server
hostebin version                            version, commit, build date
```

### up flags

| Flag | Meaning |
| --- | --- |
| `--title TEXT` | Bundle title, shown in listings and generated pages |
| `--ttl DURATION` | `30m`, `12h`, `7d`, `2w`, or `never` |
| `--entry PATH` | File served at the bundle root |
| `--id ID` | Replace an existing bundle in place (same URL) |
| `-n, --name PATH` | Name given to data read from stdin; required with `-` |
| `--json` | Print the full JSON response instead of the URL |
| `--open` | Open the URL in the system browser |
| `--quiet` | Suppress optional diagnostics |
| `--server URL` | Override `HOSTEBIN_SERVER` / config |
| `--token TOKEN` | Override `HOSTEBIN_TOKEN` / config |
| `--config PATH` | Use a different config file |

Upload naming rules: directory arguments contribute paths relative to that
directory; absolute or escaping paths collapse to their base name; duplicate names,
symlinks, and non-regular files are refused; `-` may appear once.

### Configuration precedence

Command-line flags, then `HOSTEBIN_*` environment variables, then the JSON config
file, then built-in defaults. Every long flag has an environment variable formed by
uppercasing it and replacing `-` with `_` (`--max-upload` → `HOSTEBIN_MAX_UPLOAD`).

Config file location (mode `0600`, created on first run):

| OS | Path |
| --- | --- |
| Linux/BSD | `${XDG_CONFIG_HOME:-~/.config}/hostebin/config.json` |
| macOS | `~/Library/Application Support/hostebin/config.json` |
| Windows | `%AppData%\hostebin\config.json` |

Data directory (`token` file lives here): `${XDG_DATA_HOME:-~/.local/share}/hostebin`
on Linux/BSD, `<user config dir>/hostebin/data` elsewhere.

## HTTP API

Every `/api/v1` route needs `Authorization: Bearer $HOSTEBIN_TOKEN`. Bundle content
under `/b/{id}/` and `/healthz` are public.

```sh
# raw single file
curl -sf -H "Authorization: Bearer $HOSTEBIN_TOKEN" \
  -H 'X-Hostebin-Filename: report.html' \
  -H 'X-Hostebin-Title: Weekly report' \
  -H 'X-Hostebin-TTL: 7d' \
  --data-binary @report.html \
  "$HOSTEBIN_SERVER/api/v1/bundles"

# multipart, several files
curl -sf -H "Authorization: Bearer $HOSTEBIN_TOKEN" \
  -F 'title=Weekly report' \
  -F 'file=@report.html;filename=report.html' \
  -F 'file=@img/chart.png;filename=img/chart.png' \
  "$HOSTEBIN_SERVER/api/v1/bundles"
```

| Route | Purpose |
| --- | --- |
| `POST /api/v1/bundles` | Create a bundle |
| `PUT /api/v1/bundles/{id}?mode=replace` | Atomically replace all files |
| `PUT /api/v1/bundles/{id}?mode=merge` | Atomically add/overwrite named files |
| `GET /api/v1/bundles` | List live bundles |
| `DELETE /api/v1/bundles/{id}` | Delete a bundle |
| `GET /b/{id}/...` | Public content; `?raw=1` disables Markdown rendering |

Metadata fields: raw uploads use the `X-Hostebin-Filename`, `X-Hostebin-Title`,
`X-Hostebin-Entry`, and `X-Hostebin-TTL` headers; multipart uploads use `title`,
`entry`, and `ttl` fields alongside `file` parts.

Create/replace responses:

```json
{
  "id": "0f3c...",
  "url": "https://host/b/0f3c.../",
  "entry_url": "https://host/b/0f3c.../report.html",
  "expires_at": null,
  "files": [{"name": "report.html", "size": 1234, "url": "https://host/b/0f3c.../report.html"}]
}
```

Status codes: `201` created, `200` updated, `204` deleted, `400` bad request,
`401` bad token, `404` missing or expired, `413` too large.

## Running a server

```sh
hostebin serve                                     # http://localhost:8080
hostebin serve --host 0.0.0.0 --port 8080 --data /var/lib/hostebin
hostebin serve --port 0 --tls-addr :8443 --tls-cert cert.pem --tls-key key.pem
hostebin serve --port 0 --acme-domain files.example.com --acme-email ops@example.com
HOSTEBIN_TS_AUTH_KEY=tskey-auth-... hostebin serve --port 0 --funnel
```

Key server flags: `--data`, `--base-url`, `--max-upload` (default `32MiB`),
`--max-files` (default `64`), `--default-ttl` (default `never`), `--csp`
(`off` disables), `--tailscale`, `--funnel`, `--ts-hostname`, `--ts-auth-key`.

On first start the server generates a token, writes it to `<data>/token` with mode
`0600`, and logs it once. Expired bundles 404 immediately and are swept at startup
and every ten minutes.

## Security model

Hosted content is untrusted: no cookie auth, `X-Content-Type-Options: nosniff`,
unknown extensions served as `application/octet-stream`, and a permissive-but-scoped
default CSP. All bundles share one origin, so an unguessable ID is the read
capability — treat every bundle URL as a secret-bearing link.
