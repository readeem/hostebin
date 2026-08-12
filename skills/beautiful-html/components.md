# Components

Parts that aren't already in a template. Each uses only the shell's tokens, so it
drops into any page and themes itself. Load this when a page needs a shape the
templates don't cover.

For the parts that *are* in the templates — callouts, code blocks with a copy
button, tables, stat tiles, filter chips, section anchors, scroll-spy contents,
SVG bar charts, sortable tables — copy them from the template file instead. They
are the authoritative versions. Icons live in `icons.md`; anything below that
uses one assumes its `<symbol>` is in the page's sprite.

## Badge

Small, high-frequency, always paired with a `-soft` background. The icon is
optional — add it when badges are the thing the reader scans for, skip it when
they are incidental. If you add one to any badge, add one to all of them.

```html
<span class="rounded-full bg-ok-soft px-2 py-0.5 text-xs font-medium text-ok">Passing</span>
<span class="rounded-full bg-warn-soft px-2 py-0.5 text-xs font-medium text-warn">Flaky</span>
<span class="rounded-full bg-bad-soft px-2 py-0.5 text-xs font-medium text-bad">Failing</span>
<span class="rounded-full bg-canvas px-2 py-0.5 text-xs font-medium text-muted ring-1 ring-line">Skipped</span>
```

```html
<span class="inline-flex items-center gap-1.5 rounded-full bg-ok-soft px-2 py-0.5 text-xs font-medium text-ok">
  <svg class="icon size-3.5" aria-hidden="true"><use href="#i-circle-check"/></svg>Passing
</span>
```

## Meter

A proportion the reader should judge at a glance. Give the number too — a bar
alone can't be read precisely.

```html
<div class="max-w-sm">
  <div class="mb-1 flex items-baseline justify-between text-sm">
    <span class="text-ink">Statement coverage</span>
    <span class="font-medium tabular-nums text-ink">78.4%</span>
  </div>
  <div class="h-2 overflow-hidden rounded-full bg-line" role="img" aria-label="78.4 percent">
    <div class="h-full rounded-full bg-accent" style="width: 78.4%"></div>
  </div>
</div>
```

## Spec list

Key–value pairs — configuration, environment, run parameters. `<dl>` so the
pairing survives screen readers and copy-paste.

```html
<dl class="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
  <dt class="text-muted">Commit</dt>   <dd class="font-mono text-ink">95d8f25</dd>
  <dt class="text-muted">Go version</dt><dd class="font-mono text-ink">1.24.4</dd>
  <dt class="text-muted">Duration</dt> <dd class="tabular-nums text-ink">4m 12s</dd>
</dl>
```

## Timeline

Ordered events with times. The rail is a border on the list, not a per-item element.

```html
<ol class="ml-1.5 space-y-5 border-l border-line">
  <li class="relative pl-6">
    <span class="absolute -left-[5px] top-2 size-2.5 rounded-full bg-accent ring-4 ring-canvas"></span>
    <p class="text-sm font-medium text-ink">Deploy started</p>
    <p class="text-xs tabular-nums text-muted">14:02:11</p>
    <p class="mt-1 text-sm/6 text-muted">Optional detail.</p>
  </li>
  <li class="relative pl-6">
    <span class="absolute -left-[5px] top-2 size-2.5 rounded-full bg-bad ring-4 ring-canvas"></span>
    <p class="text-sm font-medium text-ink">Health check failed</p>
    <p class="text-xs tabular-nums text-muted">14:04:38</p>
  </li>
</ol>
```

## Side by side

Before/after, two options, two runs. Stacks on narrow screens so neither column
gets squeezed to nothing.

```html
<div class="grid gap-4 md:grid-cols-2">
  <div class="rounded-xl border border-line bg-surface">
    <p class="border-b border-line px-4 py-2 text-xs font-semibold tracking-wide text-muted uppercase">Before</p>
    <pre class="overflow-x-auto px-4 py-3 font-mono text-xs/6"><code>old</code></pre>
  </div>
  <div class="rounded-xl border border-line bg-surface">
    <p class="border-b border-line px-4 py-2 text-xs font-semibold tracking-wide text-muted uppercase">After</p>
    <pre class="overflow-x-auto px-4 py-3 font-mono text-xs/6"><code>new</code></pre>
  </div>
</div>
```

## Diff

Paste the raw patch and let the highlighter tint it; see *Code and diffs* in
`SKILL.md`, and copy the script from `templates/report.html`.

```html
<pre data-lang="go" data-diff class="overflow-x-auto py-3.5 font-mono text-xs/6"><code>@@ -12,3 +12,3 @@
 	func serve() {
-		log.Print("v1")
+		log.Print("v2")</code></pre>
```

## Figure

An image or screenshot with a caption. `max-w-full` keeps it inside the measure;
the caption says what to look at.

```html
<figure class="my-6">
  <img src="chart.png" alt="Describe what the image shows, not that it is an image."
       class="max-w-full rounded-xl border border-line">
  <figcaption class="mt-2 text-sm text-muted">What the reader should notice in it.</figcaption>
</figure>
```

Upload the image alongside the page so the relative path resolves:
`hostebin up report.html chart.png`.

## Empty state

Shown when a filter or a query matches nothing. Say what would fill it.

```html
<p class="rounded-xl border border-dashed border-line px-4 py-10 text-center text-sm text-muted">
  No findings at this severity.
</p>
```

## Sticky table header

For tables long enough to scroll past their own header.

```html
<div class="max-h-[70vh] overflow-auto rounded-xl border border-line">
  <table class="w-full border-collapse text-sm">
    <thead class="sticky top-0 bg-surface text-left shadow-[0_1px_0_var(--line)]">
      <tr><th class="px-4 py-2.5 font-semibold">Name</th></tr>
    </thead>
    <tbody class="divide-y divide-line"><!-- rows --></tbody>
  </table>
</div>
```

## Tabs

Only when two views are genuinely alternatives. Panels are in the HTML, so with
JS off the reader sees all of them stacked rather than none.

```html
<div class="tabs">
  <div class="mb-4 flex gap-1 border-b border-line" role="tablist">
    <button role="tab" aria-selected="true"  aria-controls="p1"
            class="-mb-px border-b-2 border-transparent px-3 py-2 text-sm text-muted aria-selected:border-accent aria-selected:font-medium aria-selected:text-ink">Summary</button>
    <button role="tab" aria-selected="false" aria-controls="p2"
            class="-mb-px border-b-2 border-transparent px-3 py-2 text-sm text-muted aria-selected:border-accent aria-selected:font-medium aria-selected:text-ink">Raw output</button>
  </div>
  <div id="p1" role="tabpanel">First panel.</div>
  <div id="p2" role="tabpanel">Second panel.</div>
</div>

<script>
for (const group of document.querySelectorAll('.tabs')) {
  const tabs = [...group.querySelectorAll('[role="tab"]')];
  const show = tab => {
    for (const t of tabs) {
      const on = t === tab;
      t.setAttribute('aria-selected', String(on));
      document.getElementById(t.getAttribute('aria-controls')).hidden = !on;
    }
  };
  for (const t of tabs) t.addEventListener('click', () => show(t));
  show(tabs[0]);
}
</script>
```

## References

Numbered sources at the foot of a report. The ids let the body link into them.

```html
<section class="mt-12 border-t border-line pt-5">
  <h2 class="mb-3 text-sm font-semibold tracking-widest text-muted uppercase">References</h2>
  <ol class="space-y-1.5 text-sm text-muted">
    <li id="ref-1">
      <span class="tabular-nums">[1]</span>
      <a href="https://example.com" class="text-accent underline decoration-accent/35 underline-offset-2">Title of the source</a>
      — what it was used for.
    </li>
  </ol>
</section>
```

Cite it inline with `<a href="#ref-1" class="text-accent">[1]</a>`.
