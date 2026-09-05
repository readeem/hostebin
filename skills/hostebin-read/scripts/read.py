#!/usr/bin/env python3
"""Fetch a Hostebin URL and print its useful text."""

from __future__ import annotations

import re
import sys
from html.parser import HTMLParser
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit
from urllib.request import Request, urlopen


BLOCK_TAGS = {
    "address",
    "article",
    "aside",
    "blockquote",
    "dd",
    "div",
    "dl",
    "dt",
    "figcaption",
    "figure",
    "footer",
    "h1",
    "h2",
    "h3",
    "h4",
    "h5",
    "h6",
    "header",
    "hr",
    "li",
    "main",
    "ol",
    "p",
    "pre",
    "section",
    "table",
    "tbody",
    "tfoot",
    "thead",
    "tr",
    "ul",
}
SKIP_TAGS = {
    "button",
    "canvas",
    "head",
    "nav",
    "noscript",
    "script",
    "style",
    "svg",
    "template",
}
VOID_TAGS = {
    "area",
    "base",
    "br",
    "col",
    "embed",
    "hr",
    "img",
    "input",
    "link",
    "meta",
    "param",
    "source",
    "track",
    "wbr",
}


class UsefulTextParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.parts: list[str] = []
        self.skip_depth = 0
        self.pre_depth = 0
        self.row_cells = 0

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        attrs_by_name = dict(attrs)
        classes = set((attrs_by_name.get("class") or "").split())
        style = re.sub(r"\s+", "", (attrs_by_name.get("style") or "").lower())
        hidden = (
            tag in SKIP_TAGS
            or "hidden" in attrs_by_name
            or attrs_by_name.get("aria-hidden", "").lower() == "true"
            or attrs_by_name.get("role", "").lower() == "navigation"
            or "no-print" in classes
            or "display:none" in style
            or "visibility:hidden" in style
        )
        if self.skip_depth or hidden:
            if tag not in VOID_TAGS:
                self.skip_depth += 1
            return

        if tag == "pre":
            self._break(2)
            self.pre_depth += 1
        elif tag == "br":
            self._break(1)
        elif tag == "li":
            self._break(1)
            self.parts.append("- ")
        elif tag == "tr":
            self._break(1)
            self.row_cells = 0
        elif tag in {"td", "th"}:
            if self.row_cells:
                self.parts.append("\t")
            self.row_cells += 1
        elif tag in BLOCK_TAGS:
            self._break(2)

        if tag == "img" and attrs_by_name.get("alt"):
            self._text(attrs_by_name["alt"] or "")

    def handle_startendtag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        self.handle_starttag(tag, attrs)
        if tag not in VOID_TAGS:
            self.handle_endtag(tag)

    def handle_endtag(self, tag: str) -> None:
        if self.skip_depth:
            self.skip_depth -= 1
            return
        if tag == "pre":
            self.pre_depth = max(0, self.pre_depth - 1)
            self._break(2)
        elif tag in {"li", "tr"}:
            self._break(1)
        elif tag in BLOCK_TAGS:
            self._break(2)

    def handle_data(self, data: str) -> None:
        if not self.skip_depth:
            self._text(data)

    def text(self) -> str:
        text = "".join(self.parts)
        text = re.sub(r" *\t *", "\t", text)
        text = re.sub(r"[ \t]+\n", "\n", text)
        text = re.sub(r"\n{3,}", "\n\n", text)
        return text.strip()

    def _text(self, data: str) -> None:
        if self.pre_depth:
            self.parts.append(data)
            return
        collapsed = re.sub(r"\s+", " ", data)
        if not collapsed:
            return
        if self.parts and self.parts[-1].endswith((" ", "\n", "\t")):
            collapsed = collapsed.lstrip()
        if collapsed:
            self.parts.append(collapsed)

    def _break(self, count: int) -> None:
        if not self.parts:
            return
        trailing = len(self.parts[-1]) - len(self.parts[-1].rstrip("\n"))
        if trailing < count:
            self.parts.append("\n" * (count - trailing))


def raw_url(url: str) -> str:
    parsed = urlsplit(url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("expected an http or https Hostebin URL")
    query = [(key, value) for key, value in parse_qsl(parsed.query, keep_blank_values=True) if key != "raw"]
    query.append(("raw", "1"))
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path, urlencode(query), ""))


def looks_like_html(text: str, final_url: str) -> bool:
    path = urlsplit(final_url).path.lower()
    if path.endswith((".html", ".htm")):
        return True
    sample = text[:8192].lstrip("\ufeff\t\r\n ").lower()
    return bool(
        re.match(
            r"(?:<!--.*?-->\s*)*(?:<!doctype\s+html|<html(?:\s|>)|<head(?:\s|>)|<body(?:\s|>))",
            sample,
            re.DOTALL,
        )
    )


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {sys.argv[0]} <hostebin-url>", file=sys.stderr)
        return 2

    try:
        request = Request(raw_url(sys.argv[1]), headers={"User-Agent": "hostebin-read/1"})
        with urlopen(request, timeout=30) as response:
            content = response.read()
            final_url = response.geturl()
    except Exception as error:
        print(f"hostebin-read: {error}", file=sys.stderr)
        return 1

    source = content.decode("utf-8-sig", errors="replace")
    if not looks_like_html(source, final_url):
        sys.stdout.write(source)
        return 0

    parser = UsefulTextParser()
    parser.feed(source)
    parser.close()
    output = parser.text()
    if output:
        print(output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
