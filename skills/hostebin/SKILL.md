---
name: hostebin
description: Publish HTML, Markdown, images, or a whole static folder to a shareable web URL with the hostebin CLI. Use when someone needs to read generated output in a browser instead of a terminal, or when an already-published link must be updated, listed, or deleted.
---

# Publishing with hostebin

`hostebin up` uploads a small bundle of files and prints one URL. Hand that URL to
the person you are working with; they open it in a browser and see the rendered
page.

## Publishing a writeup

1. **Write the page.** The `beautiful-html` skill has the templates and the layout
   rules for a document worth reading.
2. **Upload it with a title and a TTL.**
   ```sh
   hostebin up --title 'Weekly report' --ttl 14d report.html
   ```
   The title is what the reader sees in the tab and in `hostebin ls`, so make it
   say what the page is. The default expiry is `never`; two weeks outlives almost
   every writeup, so pass `--ttl` unless the artifact is meant to be permanent.
3. **Hand back the printed line verbatim.** A bundle renders at its root, and
   `hostebin up` prints exactly that URL:

   > ✅ `https://762a1b8266153f8ba48b.hostebin.example.com/`
   >
   > ❌ `https://762a1b8266153f8ba48b.hostebin.example.com/page.html`

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
```

The URL is the only thing on stdout; everything else goes to stderr, so capture it
directly:

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
- **Update in place while iterating**, so the link you already gave the person keeps
  working:
  ```sh
  hostebin up --id <bundle-id> report.html
  ```

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

Full flag list, user and token commands, config precedence, HTTP API, and the
security model: `reference.md` next to this file.
