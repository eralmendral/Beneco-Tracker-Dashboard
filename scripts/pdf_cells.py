"""Coordinate-based PDF table cell extraction helpers."""

from typing import Any


def extract_cell_text(page: Any, bounds: tuple[float, float, float, float]) -> str:
    left, top, right, bottom = bounds
    chars = [
        char
        for char in page.chars
        if left <= (float(char["x0"]) + float(char["x1"])) / 2 < right
        and top <= (float(char["top"]) + float(char["bottom"])) / 2 < bottom
    ]
    lines: list[list[dict[str, Any]]] = []
    for char in sorted(chars, key=lambda item: (float(item["top"]), float(item["x0"]))):
        if not lines or abs(float(char["top"]) - float(lines[-1][0]["top"])) > 2:
            lines.append([char])
        else:
            lines[-1].append(char)

    text_lines: list[str] = []
    for line in lines:
        parts: list[str] = []
        previous_x1: float | None = None
        for char in sorted(line, key=lambda item: float(item["x0"])):
            if previous_x1 is not None and float(char["x0"]) - previous_x1 > 1:
                parts.append(" ")
            parts.append(str(char["text"]))
            previous_x1 = float(char["x1"])
        text_lines.append("".join(parts))
    return " ".join(" ".join(text_lines).split())
