---
name: beautiful-html
description: Produce perfectly readable HTML pages. Use before writing any HTML meant to be opened as a link.
---

# Start from a template

Copy the closest one, then edit to speed up your work.

| Template | Use for |
| --- | --- |
| `templates/report.html` | Plans, research, design docs, prose reviews — anything mostly read top to bottom. Sticky contents, callouts, code blocks, tables. |
| `templates/findings.html` | Code review, audits, triage, test results — many small items with a severity and a location. Filter chips and text search. |
| `templates/dashboard.html` | Benchmarks, log summaries, run statistics — headline numbers, a chart, and the table behind them. |
| `templates/shell.html` | Anything else. The `<head>`, a header, and an empty body. |

```sh
cp skills/beautiful-html/templates/report.html /tmp/page.html
```

Then work through it: replace the content, delete the sections you don't need, and
pull anything extra from the two reference files next to this one —
`components.md` for shapes the templates don't cover, `icons.md` for the icon
sprite and the rest of the set.

## The shell

Everything between `<head>` and `</head>` is fixed. Paste it verbatim; seven things
depend on each other in it.

- **Tailwind v4 loads from `cdn.jsdelivr.net`.** hostebin's default CSP allows
  `https:` scripts, so it compiles in the browser with no build step.
- **`connect-src` is `'self' https:`.** A page *may* fetch from an HTTPS origin at
  runtime, so a live API is available — see *Live data* below. Plain HTTP is not,
  deliberately: it stops a published page probing the reader's own network.
- **The favicon** is an inline `data:` SVG, so the tab icon needs no second file.
- **The pre-paint script** sets the theme class before the body renders, so the page
  never flashes light before going dark.
- **The inline base style** gives the page a background, colour, and font of its own,
  so it stays readable in the moment before Tailwind compiles — and if the CDN never
  answers at all.
- **`@custom-variant dark`** makes dark mode a class, which is what lets the toggle
  work while still defaulting to the reader's system setting.
- **The `.icon` rule** in `@layer components` is the only place icon size, stroke,
  and alignment are set, so every `<use href="#i-…">` on the page matches.

## Tokens

Colour comes from named tokens, not from Tailwind's palette. Each one already has
its dark value, so `bg-surface` is correct in both themes and there is no `dark:`
variant to forget.

| Token | Is |
| --- | --- |
| `canvas` | The page background |
| `surface` | Cards, table headers, anything sitting on the canvas |
| `ink` | Body text |
| `muted` | Labels, captions, secondary text |
| `line` | Borders and dividers |
| `code` | Code backgrounds |
| `accent` | Links, active state, the one chart series |
| `ok` `warn` `bad` | Status, and only status |

Each of `accent`, `ok`, `warn`, `bad` has a `-soft` partner for backgrounds:
`text-bad` on `bg-bad-soft`. Every pairing in that table clears WCAG AA in both
themes — reach for `text-red-500` instead and you give that up.

Retheme a whole page by editing the `:root` and `.dark` blocks. Nothing else.

## Rules

- **Cap prose at 68 characters.** `max-w-[68ch]` on text; full width is for tables,
  charts, and code. Past about 75 characters the eye loses the next line.
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

## Live data

`connect-src` allows any HTTPS origin, so a page can call an API and render what
comes back. Three things follow from that, and the first is the one that matters.

- **Inline the data anyway, when you have it.** A report of numbers you already
  know should ship those numbers, not fetch them — it then renders identically in
  a year, offline, and in print. Reach for `fetch` when the page's job is to be
  *current*, not to save yourself pasting a dataset. `dashboard.html` inlines its
  series in a `<script type="application/json">` for exactly this reason.
- **Never put a credential in the page.** Reads are public: anyone with the link
  has the key, and links get pasted into issues and chats. Only unauthenticated,
  CORS-enabled endpoints belong here.
- **Render something useful when the call fails**, because it will — CORS, an
  expired endpoint, a reader offline. Ship the last known values in the HTML and
  let the fetch replace them, or show a plain "couldn't load, here's what we knew
  at 14:02" line. A permanent spinner is the one failure mode worse than static
  data.

Plain HTTP is blocked, so `http://localhost:…` and LAN addresses will not work
and should not be attempted.

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

- **Take them from the sprite, not from a CDN.** Icon fonts and Iconify both work
  under the current CSP, and both make the page's chrome wait on a network round
  trip that the rest of the page doesn't need — a slow CDN shows blank boxes where
  the severity markers should be. The sprite has none of that and costs 24 lines.
  `icons.md` has the full set and what each alternative actually costs.
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

Every template ships a favicon as an inline `data:` SVG — no second file, no
request. **Change its colour on every page you publish.** The reader keeps
several of these open at once, and a tab is about fifteen characters of title
plus an icon; if the icon never changes, the icon is doing nothing.

`icons.md` has the encoding rules and a glyph-and-hue table to pick from. The
one thing that bites: `#` must be written `%23`, and a mistake anywhere in the
URI produces a blank tab icon with no error.

## Behaviour

Scripts are enhancements. The page is complete in HTML; JavaScript only adds
affordances, so a broken script costs a convenience rather than the content.

Worth writing: theme toggle, copy-to-clipboard, filter and search, sortable table,
scroll-spy contents, `<details>` disclosure, back-to-top. Reach past those and the
page has stopped being a document.

- One `<script>` at the end of `<body>`, plain DOM, no framework.
- Guard every lookup: `document.getElementById('x')?.addEventListener(...)`.
- **Name top-level variables distinctly** — `toTop`, not `top`. A top-level
  `const top` collides with the non-configurable `window.top` and throws before a
  single line of the script runs, silently killing every behaviour on the page.
