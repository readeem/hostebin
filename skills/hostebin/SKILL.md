---
name: hostebin
description: Publish HTML pages, Markdown reports, charts, images, or whole static folders to a shareable web URL with the hostebin CLI. Use whenever a person needs to *view* generated output in a browser instead of reading it in a terminal, or when a previously shared link must be updated, listed, or deleted.
---

# Publishing with hostebin

`hostebin` uploads a small bundle of files and prints one URL. Hand that URL to the
person you are working with; they open it in a browser and see the rendered page.

Use it when the work product is visual or too large for chat: a report, a chart, a
diff view, a generated site, a screenshot, a long Markdown document.

For templates and advanced guidance on writing a document to publish here use the `beautiful-html-reports` skill.

## Uploading

```sh
# single page
hostebin up report.html

# a page plus its assets — relative links keep working
hostebin up report.html img/chart.png styles/report.css

# a whole directory (the directory name itself is stripped)
hostebin up ./site/

# generated content, no temp file needed
printf '# Status\n\nAll green.\n' | hostebin up -n status.md -

# expiring link with a title
hostebin up --title 'Weekly report' --ttl 7d report.html
```

`hostebin up` writes exactly one line to stdout: the URL. Everything else goes to
stderr. Capture it directly:

```sh
url=$(hostebin up report.html)
```

Use `--json` when you need the bundle ID, per-file URLs, or the expiry:

```sh
hostebin up --json report.html
# {"id":"...","url":"https://.../b/<id>/","entry_url":"...","files":[...],"expires_at":null}
```

## Rules that matter

- **Markdown is rendered** as a styled HTML page; add `?raw=1` to the URL for source.
- **The entry page** is picked automatically: a single file, else `index.html`, else
  the first HTML file, else the first Markdown file, else a generated file listing.
  Override with `--entry path/to/file.html`.
- **Self-contained output wins.** Inline CSS/JS or upload assets alongside the page;
  bundles cannot reference files from other bundles.
- **Reads are public.** Anyone with the link can read it. Never upload secrets,
  credentials, tokens, private keys, or personal data. Say so if the user asks you
  to publish something that looks sensitive.
- **Prefer updating over re-uploading** when iterating on the same artifact, so the
  link you already gave the person keeps working:
  ```sh
  hostebin up --id <bundle-id> report.html
  ```
- **Default expiry is never.** Use `--ttl 24h` / `--ttl 7d` for throwaway output.

## Managing what you published

```sh
hostebin ls                # id, bytes, created, title — one bundle per line
hostebin ls --json         # full metadata
hostebin rm <id>           # delete
```

## When something fails

| Symptom | Cause and fix |
| --- | --- |
| `server URL is required` / `token is required` | `HOSTEBIN_SERVER` / `HOSTEBIN_TOKEN` unset, and no config file. |
| `server returned 401` | Wrong token; re-read it from the server's data directory. |
| `server returned 413` | Bundle exceeds the server's `--max-upload` (default 32 MiB). |
| `upload contains more than N files` | More than `--max-files` (default 64); upload a subset or zip assets. |
| `refusing symlink` | Directory contains symlinks; upload the real files. |
| connection refused | No server running at `HOSTEBIN_SERVER`. |

Exit codes: `0` success, `1` usage/configuration error, `2` network or server error.

## More detail

Full flag list, HTTP API, and server setup: see `reference.md` next to this file.
