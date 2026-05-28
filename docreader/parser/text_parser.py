"""Plain text and CSV parsers for the built-in DocReader engine."""

from __future__ import annotations

import csv
from io import StringIO

from docreader.models.document import Document
from docreader.parser.base_parser import BaseParser


class TextParser(BaseParser):
    """Parse UTF-8-ish text bytes into a Document."""

    def parse_into_text(self, content: bytes) -> Document:
        return Document(content=_decode_text(content))


class CSVParser(BaseParser):
    """Parse CSV bytes into a Markdown table."""

    def parse_into_text(self, content: bytes) -> Document:
        text = _decode_text(content)
        rows = list(csv.reader(StringIO(text)))
        if not rows:
            return Document(content="")

        width = max(len(row) for row in rows)
        normalized = [row + [""] * (width - len(row)) for row in rows]
        header = normalized[0]
        body = normalized[1:]

        lines = [
            "| " + " | ".join(_escape_cell(cell) for cell in header) + " |",
            "|" + "|".join(" --- " for _ in header) + "|",
        ]
        for row in body:
            lines.append("| " + " | ".join(_escape_cell(cell) for cell in row) + " |")
        return Document(content="\n".join(lines) + "\n")


def _decode_text(content: bytes) -> str:
    for encoding in ("utf-8-sig", "utf-8", "cp949", "euc-kr"):
        try:
            return content.decode(encoding)
        except UnicodeDecodeError:
            continue
    return content.decode("utf-8", errors="replace")


def _escape_cell(value: str) -> str:
    return value.replace("|", "\\|").replace("\n", "<br>")
