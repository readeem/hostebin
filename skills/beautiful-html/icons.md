# Icons

An icon on a report page has one job: let the eye find a row before it reads the
row. It labels — severity, direction, state, the action a button performs. It
never decorates a heading.

## Pick a way to get them onto the page

hostebin's default CSP is
`default-src 'self' data: blob: https: 'unsafe-inline' 'unsafe-eval'; connect-src 'self' https:`.
Every approach below is *permitted* by it. They differ in what they cost, and the
cost that matters is that icons are chrome: they should not be the reason a
finished-looking page is missing its severity markers for two seconds.

| Approach | Verdict |
| --- | --- |
| **Inline `<symbol>` sprite + `<use>`** | **Use this.** No network, no JS, one definition per icon however often it repeats, and `currentColor` themes it for free. |
| Inline `<svg>` per occurrence | Fine for an icon used once. Past twice you are pasting path data around; move it to the sprite. |
| Icon font from a CDN (Material Symbols, Font Awesome) | Loads, but it is a network dependency for chrome. When the CDN is slow the reader gets blank boxes, and a ligature font shows the raw name (`check_circle`) until it resolves. |
| Lucide/Feather UMD script + `createIcons()` | Works, and the icon data ships inside the bundle so there is no second request — but it costs ~1 MB and the icons vanish with JS off, which contradicts every other behaviour rule here. |
| Iconify, or any runtime icon API | Now works — `connect-src` allows HTTPS — but it puts a live API call between the reader and a check mark, and the page renders with holes until it answers. |
| SVG in a CSS `background-image: url("data:image/svg+xml,…")` | Loads (`data:` is allowed) but the fill is baked in, so it cannot follow the theme. Only for something genuinely decorative. |
| Emoji | No. |

Note that plain HTTP is *not* allowed — `connect-src` is `https:`, not `*` — so
nothing here can be sourced from `http://` or a LAN address.

## The sprite

Paste this right after `<body>`, keeping only the symbols the page references.
The container is `display:none` — a `<use>` still resolves into it.

```html
<svg style="display:none" aria-hidden="true">
  <symbol id="i-circle-check" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/></symbol>
  <symbol id="i-triangle-alert" viewBox="0 0 24 24"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/></symbol>
</svg>
```

The shell's `@layer components` already carries the one rule that styles every
one of them:

```css
.icon { @apply inline-block size-4 shrink-0 align-[-0.125em];
        fill: none; stroke: currentColor; stroke-width: 2;
        stroke-linecap: round; stroke-linejoin: round; }
```

`fill: none; stroke: currentColor` is what makes an icon inherit `text-bad` or
`text-muted` from whatever it sits in, in both themes, with no per-icon colour.

Use one:

```html
<!-- Next to a text label: the text is the label, so hide the icon. -->
<span class="inline-flex items-center gap-1.5 text-bad">
  <svg class="icon size-3.5" aria-hidden="true"><use href="#i-circle-x"/></svg>High
</span>

<!-- Alone in a button: the icon carries the meaning, so name it. -->
<button type="button" aria-label="Back to top">
  <svg class="icon" aria-hidden="true"><use href="#i-arrow-up"/></svg>
</button>

<!-- Alone in content, with no adjacent text: give it a title. -->
<svg class="icon text-ok" role="img"><title>Passing</title><use href="#i-circle-check"/></svg>
```

Override the size with a utility — `size-3.5` inside a badge, `size-4` in body
text, `size-5` in a tile. The stroke stays 2 user units and thins with the box,
which is what keeps a small icon from looking heavier than the text beside it.

## The set

[Lucide](https://lucide.dev) (ISC). Every symbol below is a 24×24 viewBox, so
they share a stroke weight and optical size. Adding one outside this list is
fine — take it from `lucide.dev`, keep the `id` prefix, and drop the wrapper
`<svg>`'s attributes, since `.icon` supplies them.

### Status and severity

```html
<symbol id="i-circle-check" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/></symbol>
<symbol id="i-triangle-alert" viewBox="0 0 24 24"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/></symbol>
<symbol id="i-circle-x" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="m15 9-6 6"/><path d="m9 9 6 6"/></symbol>
<symbol id="i-circle-alert" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/></symbol>
<symbol id="i-info" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></symbol>
<symbol id="i-circle-help" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><path d="M12 17h.01"/></symbol>
<symbol id="i-minus" viewBox="0 0 24 24"><path d="M5 12h14"/></symbol>
```

`circle-check` / `triangle-alert` / `circle-x` map onto `ok` / `warn` / `bad`
and should not be used for anything else — that pairing is the whole reason a
reader can scan the page. `info` is the neutral fourth.

### Direction and change

```html
<symbol id="i-trending-up" viewBox="0 0 24 24"><path d="M16 7h6v6"/><path d="m22 7-8.5 8.5-5-5L2 17"/></symbol>
<symbol id="i-trending-down" viewBox="0 0 24 24"><path d="M16 17h6v-6"/><path d="m22 17-8.5-8.5-5 5L2 7"/></symbol>
<symbol id="i-arrow-up" viewBox="0 0 24 24"><path d="m5 12 7-7 7 7"/><path d="M12 19V5"/></symbol>
<symbol id="i-chevron-up" viewBox="0 0 24 24"><path d="m18 15-6-6-6 6"/></symbol>
<symbol id="i-chevron-down" viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></symbol>
<symbol id="i-chevron-right" viewBox="0 0 24 24"><path d="m9 18 6-6-6-6"/></symbol>
<symbol id="i-chevrons-up-down" viewBox="0 0 24 24"><path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/></symbol>
```

Up is not automatically good. A latency chart that trends up gets
`text-bad`; colour carries the judgement, the arrow carries only the direction.

### Chrome

```html
<symbol id="i-sun" viewBox="0 0 24 24"><circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/></symbol>
<symbol id="i-moon" viewBox="0 0 24 24"><path d="M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401"/></symbol>
<symbol id="i-copy" viewBox="0 0 24 24"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></symbol>
<symbol id="i-check" viewBox="0 0 24 24"><path d="M20 6 9 17l-5-5"/></symbol>
<symbol id="i-search" viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.34-4.34"/></symbol>
<symbol id="i-x" viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></symbol>
<symbol id="i-external-link" viewBox="0 0 24 24"><path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/></symbol>
<symbol id="i-list-tree" viewBox="0 0 24 24"><path d="M8 5h13"/><path d="M13 12h8"/><path d="M13 19h8"/><path d="M3 10a2 2 0 0 0 2 2h3"/><path d="M3 5v12a2 2 0 0 0 2 2h3"/></symbol>
```

### Subject matter

```html
<symbol id="i-clock" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></symbol>
<symbol id="i-file-text" viewBox="0 0 24 24"><path d="M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z"/><path d="M14 2v5a1 1 0 0 0 1 1h5"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></symbol>
<symbol id="i-git-commit-horizontal" viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M3 12h6"/><path d="M15 12h6"/></symbol>
<symbol id="i-zap" viewBox="0 0 24 24"><path d="M15.914 4a1.5 1.5 0 0 0-2.474-1.561l-9 9A1.5 1.5 0 0 0 5.5 14h4.002a.5.5 0 0 1 .471.666L8.086 20a1.5 1.5 0 0 0 2.475 1.56l9-9A1.5 1.5 0 0 0 18.5 10h-3.997a.5.5 0 0 1-.472-.667z"/></symbol>
<symbol id="i-gauge" viewBox="0 0 24 24"><path d="m12 14 4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/></symbol>
<symbol id="i-flask-conical" viewBox="0 0 24 24"><path d="M14 2v6a2 2 0 0 0 .245.96l5.51 10.08A2 2 0 0 1 18 22H6a2 2 0 0 1-1.755-2.96l5.51-10.08A2 2 0 0 0 10 8V2"/><path d="M6.453 15h11.094"/><path d="M8.5 2h7"/></symbol>
<symbol id="i-lightbulb" viewBox="0 0 24 24"><path d="M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5"/><path d="M9 18h6"/><path d="M10 22h4"/></symbol>
```

## Where they belong

Use one when it is the thing the eye searches for:

- **Severity and status**, beside the word — badges, callouts, finding rails.
- **Direction of change**, beside a delta — `+22%` reads faster with an arrow.
- **The action a control performs** — copy, search, clear, back to top, theme.
- **Disclosure state** — a chevron that rotates when a `<details>` opens.
- **Sort state** on a table header.

And not:

- Beside a heading. A heading is already the thing that finds itself.
- One per bullet in a list. That is a list with a texture problem, not an icon
  problem.
- As the only carrier of meaning in a table cell. Give it a text label or a
  `<title>`; a lone glyph is unreadable to a screen reader and unsearchable to
  Ctrl-F.
- More than one per row. Two icons competing is worse than none.

Rules of thumb: no more than about six icon *instances* on a screenful, and no
more than four distinct icons in a page's whole vocabulary. Past that they stop
being landmarks and become wallpaper.

## Favicons

The reader will have several of these open at once. A tab that shows the generic
document icon is one they have to read the title to identify, and the title is
truncated to about fifteen characters. Give every page a favicon, and make it
differ from the last one you published.

Every template already ships one: a Lucide glyph as an inline `data:` URI, so it
costs no second file and no request, and it survives being uploaded on its own.

```html
<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http%3A//www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%234f7fd4' stroke-width='2.5' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M3 3v16a2 2 0 0 0 2 2h16'/%3E%3Cpath d='M18 17V9'/%3E%3Cpath d='M13 17V5'/%3E%3Cpath d='M8 17v-3'/%3E%3C/svg%3E">
```

To change it, take a glyph from the set above and rewrite two things:

- **The colour**, in `stroke='%23RRGGBB'` — `#` must be written `%23`.
- **The paths**, which are the same `<path>` elements as the sprite symbols.

Everything else stays. The encoding rules that matter: `<` and `>` become `%3C`
and `%3E`, the `:` in the xmlns becomes `%3A`, attributes inside the SVG use
single quotes, and the whole URI goes in double quotes. Getting one wrong gives
you a blank tab icon and no error anywhere.

`stroke-width` is `2.5` rather than the `2` used in the page, because at 16px a
2-unit stroke goes thin and muddy.

### Picking one

| Report | Glyph | Colour |
| --- | --- | --- |
| Plan, research, design doc | `file-text` | `%234f7fd4` blue |
| Review, audit, findings | `list-checks` | `%238b5cf6` violet |
| Benchmarks, metrics, run stats | `chart-column` | `%230d9488` teal |
| Incident, postmortem | `triangle-alert` | `%23e0532f` red |
| Anything else | `file-text` | `%2364748b` slate |

Use the colour as the real signal — glyphs at 16px are hard to tell apart, hue
is not. If you are publishing a second report of the same kind, change the hue
anyway; two identical tabs is the problem you are solving.

One caveat worth knowing: a mid-tone colour is deliberate, since the same icon
sits on a light tab bar for one reader and a dark one for another. Very light or
very dark strokes disappear on one of them.

## Rotating a chevron

The disclosure pattern, with no JavaScript:

```html
<details class="group rounded-xl border border-line bg-surface px-4 py-3">
  <summary class="flex cursor-pointer list-none items-center gap-2 text-sm font-medium select-none [&::-webkit-details-marker]:hidden">
    <svg class="icon size-4 text-muted transition-transform group-open:rotate-90" aria-hidden="true"><use href="#i-chevron-right"/></svg>
    Failure scenario
  </summary>
  <div class="mt-3 border-t border-line pt-3 text-sm/6">Detail.</div>
</details>
```

`list-none` plus the `::-webkit-details-marker` rule removes the browser's own
triangle; without both you get two markers.
