# hostebin reference

Complete surface of the CLI and HTTP API. Load this when the basic `up`/`ls`/`rm`
flow in `SKILL.md` is not enough — scripting against the API, managing users and
tokens, running a server, or debugging configuration.

## Commands

```text
hostebin up [flags] <file|directory|->...   upload; prints one URL
hostebin ls [flags]                         list live bundles; --all covers every user (admin)
hostebin rm [flags] <id>                    delete a bundle
hostebin user ls|add|rm|disable|enable      manage users
hostebin token new|rm                       rotate or revoke a token
hostebin whoami [--json]                    show the current identity
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

Data directory (`users.json`, bundles, and the legacy `token` file live here):
`${XDG_DATA_HOME:-~/.local/share}/hostebin`
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
| `GET /api/v1/bundles?all=1` | Admin global view, including `owner_id` |
| `DELETE /api/v1/bundles/{id}` | Delete a bundle |
| `GET /api/v1/whoami` | Current user, role, token ID, and token label |
| `GET /api/v1/users` | Users and token metadata; admin only |
| `POST /api/v1/users` | Create `{name, role, label, ttl}` and return the first token once; admin only |
| `PATCH /api/v1/users/{id}` | Set `{disabled}`; admin only |
| `DELETE /api/v1/users/{id}` | Delete; use `?bundles=delete|reassign` when needed |
| `PUT /api/v1/users/{id}/token` | Atomically replace the token, optional `{label, ttl}`; admin or self |
| `DELETE /api/v1/users/{id}/token` | Revoke the token; admin or self |
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

## Security model

Hosted content is untrusted: no cookie auth, `X-Content-Type-Options: nosniff`,
unknown extensions served as `application/octet-stream`, and a permissive-but-scoped
default CSP. All bundles share one origin, so an unguessable ID is the read
capability — treat every bundle URL as a secret-bearing link. Ownership scopes list,
replace, and delete operations for regular users; admins reach every bundle. Reads stay
public and unauthenticated.

Each user has at most one bearer token. Creating a token atomically replaces the
current token, and the old value fails authentication on the next request.
