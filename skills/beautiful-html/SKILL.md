---
name: beautiful-html
description: Design HTML pages worth reading — reports, plans, research writeups, reviews, dashboards. Use before writing any HTML meant to be opened as a link.
---

# Beautiful HTML

A page built from these parts reads well in both themes, prints, and survives a
dead CDN. Publish the finished file with the `hostebin` skill.

When using a template, you generally don't need to verify the HTML. Once `hostebin`
returns a URL, report it without reopening it; a sound local page is sufficient.

## Start from a template

Copy the closest one out of `templates/` next to this file into a scratch path
(`/tmp/page.html`), then edit it down.

| Template | Use for |
| --- | --- |
| `templates/report.html` | Plans, research, design docs, prose reviews — anything mostly read top to bottom. Sticky contents, callouts, code blocks, tables. |
| `templates/findings.html` | Code review, audits, triage, test results — many small items with a severity and a location. Filter chips and text search. |
| `templates/dashboard.html` | Benchmarks, log summaries, run statistics — headline numbers, a chart, and the table behind them. |
| `templates/shell.html` | Anything else. The `<head>`, a header, and an empty body. |

Then work through it: replace the content, delete the sections you don't need, and
pull anything extra from the two reference files next to this one —
`components.md` for shapes the templates don't cover, `icons.md` for the icon
sprite and the rest of the set.

The templates carry the markup and nothing else. The reasoning behind every choice
in them lives here, so a template stays short enough to read in one pass.

## The shell

Everything between `<head>` and `</head>` is the same in every template apart from
the title, the favicon, and a few per-template component rules (`.tile`, `.chip`).
Paste it verbatim; the pieces depend on each other.

- **The favicon is required.** See *Favicons* below — it is the one part of the
  shell that must change per page rather than be pasted unchanged.
- **Tailwind v4 loads from `cdn.jsdelivr.net`.** hostebin's default CSP allows
  `https:` scripts, so it compiles in the browser with no build step. When the CDN
  is slow the page holds the fallback style below until it answers.
- **Highlight.js 11.11.1 loads from `cdn.jsdelivr.net`.** It highlights explicit
  `language-*` code blocks after the document parses; code stays readable if it
  never loads.
- **DM Sans and JetBrains Mono load from Google Fonts**, mapped to `--font-sans`
  and `--font-mono`. Both degrade to system faces.
- **`connect-src` is `'self' https:`.** A page *may* fetch from an HTTPS origin at
  runtime, so a live API is available — see *Live data* below.
- **The pre-paint script** sets the theme class before the body renders, so the page
  never flashes light before going dark. It is **dark unless the reader has chosen
  light**, rather than following the system setting: these pages are dark-first.
- **The inline base style** gives the page a background, colour, and font of its own,
  so it stays readable in the moment before Tailwind compiles — and if the CDN never
  answers at all.
- **`@custom-variant dark`** makes dark mode a class, which is what lets the toggle
  work.
- **The `.icon` rule** in `@layer components` is the only place icon size, stroke,
  and alignment are set, so every `<use href="#i-…">` on the page matches.
- **The `.hljs-*` rules** colour code and diffs — see *Code and diffs*.

## Tokens

Colour comes from named tokens, not from Tailwind's palette. Each one already has
its dark value, so `bg-surface` is correct in both themes and there is no `dark:`
variant to forget. The dark values are T3 Code's own, read out of the app, with the
canvas pushed to true black.

| Token | Is | Dark |
| --- | --- | --- |
| `canvas` | The page background | `#000` |
| `surface` | Cards, tables, code blocks, anything on the canvas | 5% white |
| `ink` | Body text | `#f5f5f5` |
| `muted` | Labels, captions, secondary text | `#818181` |
| `line` | Borders and dividers | 9% white |
| `code` | Inline code, chips, hover fills | 5% white |
| `accent` | Links, active state, the one chart series | `oklch(65% .21 264)` |
| `ok` `warn` `bad` | Status, and only status | emerald / amber / red |

Each of `accent`, `ok`, `warn`, `bad` has a `-soft` partner for backgrounds,
derived from it by `color-mix`: `text-bad` on `bg-bad-soft`. Every pairing in that
table clears WCAG AA in both themes — reach for `text-red-500` instead and you give
that up.

Retheme a whole page by editing the `:root` and `.dark` blocks. Nothing else.

## Rules

- **Every page ships a favicon.** No exceptions — see *Favicons*.
- **The heading carries the finding, not the topic.** "Bundle expiry drops links a
  day early", not "Bundle expiry". It runs to 66px because it is the one thing a
  reader who scrolls no further should leave with. Under it: one standfirst
  paragraph, then a hairline row of the numbers that matter.
- **Cap prose at 68 characters.** The templates use `max-w-[66ch]`; full width is
  for tables, charts, and code. Past about 75 the eye loses the next line.
- **Three type sizes in the body.** Size and weight carry the hierarchy; the
  templates set the scale already.
- **Accent means something.** One hue, spent on links and active state. Colour that
  decorates stops signalling.
- **Density is a feature.** Whitespace separates sections, not every line. A report
  that fits on two screens beats one that fits on six.
- **Semantic elements.** `<article>`, `<section id>`, `<table>`, `<details>`,
  `<figure>`. They are what makes Ctrl-F, printing, and screen readers work.
- **Ids on every heading you'd want to link to**, hard-coded in the markup so the
  deep link survives with JS off.
- **Wrap tables in `overflow-x-auto`.** It is the one element that reliably breaks a
  narrow screen.
- **Mark chrome `no-print`.** Reports get printed and turned into PDFs; the theme
  toggle and filter bar should not follow them there.
- **One self-contained `.html`.** Inline the CSS and JS. Images are the only thing
  worth uploading alongside.

For anything chart-shaped: one series in `accent`, hand-rolled SVG, and the table
of numbers underneath it. `dashboard.html` has a working example. A chart that
needs a second hue usually wanted to be a table.

## Code and diffs

`report.html` and `findings.html` load Highlight.js 11.11.1 from jsDelivr. Copy
the CDN script and local `.hljs-*` theme rules across with any code you paste,
then use the standard `language-*` class on `<code>`:

```html
<pre><code class="language-go">func main() { … }</code></pre>
<pre><code class="language-diff">@@ -218,7 +218,7 @@
-	exp := now.Add(ttl).Truncate(24 * time.Hour)
+	exp := now.Add(ttl)</code></pre>
```

- **`language-*`** chooses a grammar explicitly: `language-go`, `language-ts`,
  `language-python`, `language-bash`, `language-sql`, `language-json`, and so on.
  The CDN build carries Highlight.js's common languages; consult its supported
  language list for names and aliases.
- **`language-diff`** tints `+` lines green, `-` lines red, and `@@` hunk headers
  muted. The markers stay in the text, so copying gives back a real patch.
- **The tint spans the scroll width**, not the visible box, which is what
  `w-max min-w-full` on `.language-diff.hljs` buys.
- **Highlighting is an enhancement.** If the CDN fails, code remains readable
  monospace and copying still returns the original text.
- Escape `<`, `>` and `&` inside `<pre>` as you would anywhere else.

## Live data

`connect-src` allows any HTTPS origin, so a page can call an API and render what
comes back. Three things follow from that, and the first is the one that matters.

- **Inline the data when you already have it.** A report of numbers you know should
  ship those numbers, so it renders identically in a year, offline, and in print.
  `fetch` is for a page whose job is to be *current*, not for saving yourself a
  paste. `dashboard.html` inlines its series in a
  `<script type="application/json">` for exactly this reason.
- **Keep credentials out of the page.** Reads are public — anyone with the link has
  the key, and links get pasted into issues and chats. Only unauthenticated,
  CORS-enabled endpoints belong here.
- **Render something useful when the call fails**, because it will — CORS, an
  expired endpoint, a reader offline. Ship the last known values in the HTML and
  let the fetch replace them, or show a plain "couldn't load, here's what we knew
  at 14:02" line. A permanent spinner is the one failure mode worse than static
  data.

Only HTTPS origins are reachable, so `http://localhost:…` and LAN addresses are
blocked by the CSP and are not worth attempting.

## Icons

Real icons, from a sprite. Every template opens its `<body>` with a
`<svg style="display:none">` block of Lucide `<symbol>`s, and one is used like
this:

```html
<svg class="icon size-3.5" aria-hidden="true"><use href="#i-circle-x"/></svg>High
```

The `.icon` rule in the shell sets `fill: none; stroke: currentColor`, so an icon
takes its colour from whatever it sits in and is correct in both themes with no
`dark:` variant. Size it with a utility: `size-3.5` in a badge, `size-4` in body
text.

- **Take them from the sprite.** It costs 24 lines, no network, and no JS, so the
  severity markers are there the moment the page paints. `icons.md` has the full
  symbol set, and what every CDN-shaped alternative costs.
- **An icon labels, it never decorates.** Severity, direction of change, the
  action a button performs, disclosure state, sort state. Not headings, not
  bullets.
- **Pair it with the token that means the same thing** — `circle-check`/`ok`,
  `triangle-alert`/`warn`, `circle-x`/`bad` — and keep that pairing for the
  whole page. Icon plus colour plus label means the severity survives being
  printed in greyscale.
- **`aria-hidden="true"` whenever text sits beside it**, so a screen reader
  doesn't read the label twice. Alone in a button, put the words in
  `aria-label`; alone in content, give the `<svg>` `role="img"` and a `<title>`.
- **Roughly six icon instances per screenful, four distinct icons per page.**
  Past that they stop being landmarks.
- **No emoji.** They are the thing this page is trying not to look like.

## Favicons

**Every page gets one.** A page published without a favicon shows the browser's
blank sheet, which is how the reader loses it among a dozen other tabs. It is the
most frequently skipped line in this skill, and it costs one line.

The templates ship it as an inline `data:` SVG — no second file, no request — so
the only work is **changing the glyph and hue per page**. A tab is about fifteen
characters of title plus an icon; if the icon never changes, the icon is doing
nothing.

`icons.md` has the encoding rules and a glyph-and-hue table to pick from. The
one thing that bites: `#` must be written `%23`, and a mistake anywhere in the
URI produces a blank tab icon with no error.

Before publishing, confirm the page has a `<link rel="icon">` whose colour is not
the one the template shipped with.

## Behaviour

Scripts are enhancements. The page is complete in HTML; JavaScript only adds
affordances, so a broken script costs a convenience rather than the content.

Worth writing: theme toggle, syntax and diff highlighting, copy-to-clipboard,
filter and search, sortable table, scroll-spy contents, `<details>` disclosure,
back-to-top. Reach past those and the page has stopped being a document.

- One custom `<script>` at the end of `<body>`, plain DOM, no framework.
- Guard every lookup: `document.getElementById('x')?.addEventListener(...)`.
- **Name top-level variables distinctly** — `toTop`, not `top`. A top-level
  `const top` collides with the non-configurable `window.top` and throws before a
  single line of the script runs, silently killing every behaviour on the page.
