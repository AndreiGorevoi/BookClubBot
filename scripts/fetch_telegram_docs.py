#!/usr/bin/env python3
"""Refresh the vendored Telegram Bot API reference.

Downloads https://core.telegram.org/bots/api and converts the documentation
body to Markdown at docs/vendor/telegram-bot-api.md, preserving the parameter
tables (the part that actually gets grepped) and the in-page #anchors.

Usage: python3 scripts/fetch_telegram_docs.py
"""

import datetime
import os
import re
import sys
import urllib.request
from html.parser import HTMLParser

SOURCE = "https://core.telegram.org/bots/api"
OUT = os.path.join(os.path.dirname(__file__), os.pardir, "docs", "vendor",
                   "telegram-bot-api.md")


def fetch(url):
    req = urllib.request.Request(url, headers={"User-Agent": "curl/8"})
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.read().decode("utf-8")


def content_body(src):
    """Slice out <div id="dev_page_content"> … </div>, honouring nesting."""
    m = re.search(r'<div id="dev_page_content"[^>]*>', src)
    if not m:
        sys.exit("dev_page_content div not found — page layout changed")
    start, depth = m.end(), 1
    for mm in re.finditer(r"<(/?)div\b[^>]*>", src[start:]):
        depth += -1 if mm.group(1) else 1
        if depth == 0:
            return src[start:start + mm.start()]
    sys.exit("unterminated dev_page_content div")


class Markdown(HTMLParser):
    """Minimal HTML→Markdown for the shape the Bot API page actually uses."""

    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.out, self.links, self.lists = [], [], []
        self.row = self.cell = self.table = None
        self.inpre = False
        self.skip = 0

    def emit(self, s):
        (self.cell if self.cell is not None else self.out).append(s)

    def handle_starttag(self, tag, attrs):
        attr = dict(attrs)
        if tag in ("script", "style"):
            self.skip += 1
        elif re.fullmatch(r"h[1-6]", tag):
            self.out.append("\n\n" + "#" * int(tag[1]) + " ")
        elif tag == "p":
            self.out.append("\n\n")
        elif tag == "br":
            self.emit("\n" if self.cell is None else " ")
        elif tag == "table":
            self.table = []
        elif tag == "tr":
            self.row = []
        elif tag in ("td", "th"):
            self.cell = []
        elif tag in ("ul", "ol"):
            self.lists.append("-" if tag == "ul" else "1.")
        elif tag == "li":
            marker = self.lists[-1] if self.lists else "-"
            self.out.append("\n" + "  " * (len(self.lists) - 1) + marker + " ")
        elif tag == "blockquote":
            self.out.append("\n\n> ")
        elif tag == "pre":
            self.inpre = True
            self.out.append("\n\n```\n")
        elif tag == "code" and not self.inpre:
            self.emit("`")
        elif tag in ("strong", "b"):
            self.emit("**")
        elif tag in ("em", "i"):
            self.emit("_")
        elif tag == "a":
            self.links.append(attr.get("href", ""))
            self.emit("[")

    def handle_endtag(self, tag):
        if tag in ("script", "style"):
            self.skip = max(0, self.skip - 1)
        elif re.fullmatch(r"h[1-6]", tag):
            self.out.append("\n")
        elif tag in ("td", "th"):
            text = re.sub(r"\s+", " ", "".join(self.cell)).strip()
            self.cell = None
            if self.row is not None:
                self.row.append(text.replace("|", "\\|"))
        elif tag == "tr":
            if self.table is not None and self.row:
                self.table.append(self.row)
            self.row = None
        elif tag == "table":
            self.flush_table()
        elif tag in ("ul", "ol"):
            if self.lists:
                self.lists.pop()
            self.out.append("\n")
        elif tag == "pre":
            self.inpre = False
            self.out.append("\n```\n")
        elif tag == "code" and not self.inpre:
            self.emit("`")
        elif tag in ("strong", "b"):
            self.emit("**")
        elif tag in ("em", "i"):
            self.emit("_")
        elif tag == "a":
            href = self.links.pop() if self.links else ""
            if href.startswith("/"):
                href = "https://core.telegram.org" + href
            self.emit("](" + href + ")")

    def flush_table(self):
        rows, self.table = self.table or [], None
        if not rows:
            return
        width = max(len(r) for r in rows)
        rows = [r + [""] * (width - len(r)) for r in rows]
        self.out.append("\n\n| " + " | ".join(rows[0]) + " |\n")
        self.out.append("|" + "|".join(["---"] * width) + "|\n")
        for r in rows[1:]:
            self.out.append("| " + " | ".join(r) + " |\n")

    def handle_data(self, data):
        if self.skip:
            return
        self.emit(data if self.inpre else re.sub(r"\s+", " ", data))


def tidy(text):
    # "#### [__](#anchor)sendMessage" -> "#### sendMessage", so headings grep.
    text = re.sub(r"^(#{1,6}) \[__\]\(#[^)]*\)\s*", r"\1 ", text, flags=re.M)
    text = re.sub(r"^>\s*$\n", "", text, flags=re.M)
    text = re.sub(r"[ \t]+\n", "\n", text)
    text = re.sub(r"\n +([#|>])", r"\1", text)
    return re.sub(r"\n{3,}", "\n\n", text).strip() + "\n"


def api_version(text):
    m = re.search(r"^#### (\w+ \d+, \d{4})\n+\*\*Bot API ([\d.]+)\*\*", text,
                  flags=re.M)
    return (m.group(2), m.group(1)) if m else ("unknown", "unknown")


def main():
    parser = Markdown()
    parser.feed(content_body(fetch(SOURCE)))
    body = tidy("".join(parser.out))
    version, released = api_version(body)
    header = f"""<!-- Generated by scripts/fetch_telegram_docs.py — do not edit by hand. -->

# Telegram Bot API — vendored reference

Mirror of <{SOURCE}>, converted to Markdown so it can be grepped offline.

- **Bot API version:** {version} (released {released})
- **Fetched:** {datetime.date.today().isoformat()}
- **Refresh:** `python3 scripts/fetch_telegram_docs.py`

The website is the source of truth. If this file disagrees with it, trust the
website and regenerate. See `docs/telegram-notes.md` for the parts that bite
this bot in particular — including the gap between this document's Bot API
version and the one the Go wrapper implements.

---

"""
    with open(OUT, "w", encoding="utf-8") as f:
        f.write(header + body)
    print(f"wrote {OUT} (Bot API {version}, {os.path.getsize(OUT)} bytes)")


if __name__ == "__main__":
    main()
