#!/usr/bin/env python3
"""Scrape BENECO's public datasets, cache them locally, and ingest snapshots."""

from __future__ import annotations

import argparse
import io
import json
import logging
import os
import re
import tempfile
from html.parser import HTMLParser
from pathlib import Path
from typing import Any
from urllib.parse import urljoin

import pdfplumber
import requests

from pdf_cells import extract_cell_text as _cell_text

BARANGAY_URL = "https://api.beneco.com.ph/cwp/barangay.php"
FORMS_URL = "https://beneco.com.ph/forms.php"
SOURCE_BARANGAYS = "barangay_feeders"
SOURCE_CONTRACTORS = "contractors"
USER_AGENT = "Beneco-Tracker-Dashboard/1.0 (+scheduled public-data archive)"


class LinkParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.links: list[tuple[str, str]] = []
        self._href: str | None = None
        self._text: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag.lower() == "a":
            self._href = dict(attrs).get("href")
            self._text = []

    def handle_data(self, data: str) -> None:
        if self._href is not None:
            self._text.append(data)

    def handle_endtag(self, tag: str) -> None:
        if tag.lower() == "a" and self._href is not None:
            self.links.append((self._href, " ".join(self._text)))
            self._href = None
            self._text = []


def discover_aep_pdf_url(html: str, base_url: str = FORMS_URL) -> str:
    parser = LinkParser()
    parser.feed(html)
    for href, text in parser.links:
        normalized = " ".join(text.split()).lower()
        if "accredited electrical practitioners" in normalized and href.lower().endswith(".pdf"):
            return urljoin(base_url, href)
    raise ValueError("could not find the accredited electrical practitioners PDF link")


def fetch_barangays(session: requests.Session, timeout: float) -> list[dict[str, Any]]:
    response = session.get(BARANGAY_URL, timeout=timeout)
    response.raise_for_status()
    payload = response.json()
    if not isinstance(payload, list) or not payload:
        raise ValueError("barangay endpoint returned no records")

    records: list[dict[str, Any]] = []
    required = ("barangayid", "barangay", "municipality", "feeder")
    for index, item in enumerate(payload):
        if not isinstance(item, dict) or any(key not in item for key in required):
            raise ValueError(f"barangay record {index} has an unexpected shape")
        try:
            barangay_id = int(item["barangayid"])
        except (TypeError, ValueError) as error:
            raise ValueError(f"barangay record {index} has an invalid barangayid") from error
        record = {
            "barangayid": barangay_id,
            "barangay": str(item["barangay"]).strip(),
            "municipality": str(item["municipality"]).strip(),
            "feeder": str(item["feeder"]).strip(),
        }
        if barangay_id <= 0 or not all(record[key] for key in required[1:]):
            raise ValueError(f"barangay record {index} is missing a required value")
        records.append(record)
    return records


def fetch_aep_pdf(session: requests.Session, timeout: float) -> tuple[str, bytes]:
    forms_response = session.get(FORMS_URL, timeout=timeout)
    forms_response.raise_for_status()
    pdf_url = discover_aep_pdf_url(forms_response.text)
    pdf_response = session.get(pdf_url, timeout=timeout)
    pdf_response.raise_for_status()
    if not pdf_response.content.startswith(b"%PDF"):
        raise ValueError("accredited-practitioner link did not return a PDF")
    return pdf_url, pdf_response.content


def _header_positions(page: Any) -> tuple[float, float, float, float, float, float]:
    words = page.extract_words(x_tolerance=1, y_tolerance=3)
    header_words = [word for word in words if word["top"] < page.height * 0.3]

    def first_x(text: str, after: float = 0.0) -> float:
        matches = [
            float(word["x0"])
            for word in header_words
            if word["text"].upper().rstrip(".") == text and float(word["x0"]) >= after
        ]
        if not matches:
            raise ValueError(f"PDF table header is missing {text}")
        return min(matches)

    name_x = first_x("NAME")
    contact_x = first_x("CONTACT", name_x)
    grade_x = first_x("GRADE", contact_x)
    license_x = first_x("LICENSE", grade_x)
    business_x = first_x("BUSINESS", license_x)
    address_x = first_x("BUSINESS", business_x + 20)
    return name_x, contact_x, grade_x, license_x, business_x, address_x


def _legacy_cell_text(page: Any, bounds: tuple[float, float, float, float]) -> str:
    text = page.crop(bounds, strict=False).extract_text(x_tolerance=1, y_tolerance=3) or ""
    return " ".join(text.split())


def parse_contractors(pdf_bytes: bytes) -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    seen_numbers: set[int] = set()

    with pdfplumber.open(io.BytesIO(pdf_bytes)) as pdf:
        if not pdf.pages:
            raise ValueError("accredited-practitioner PDF has no pages")
        columns = _header_positions(pdf.pages[0])

        for page in pdf.pages:
            name_x, contact_x, grade_x, license_x, business_x, address_x = columns
            words = page.extract_words(x_tolerance=1, y_tolerance=3)
            header_bottom = max(
                (float(word["bottom"]) for word in words if word["text"].upper().rstrip(".") in {
                    "NO", "NAME", "CONTACT", "NUMBER", "GRADE", "LICENSE", "BUSINESS", "TRADE", "ADDRESS"
                } and float(word["top"]) < page.height * 0.3),
                default=0.0,
            )
            anchors = sorted(
                (
                    (int(match.group(1)), float(word["top"]))
                    for word in words
                    if float(word["x0"]) < name_x
                    and float(word["top"]) > header_bottom
                    and (match := re.fullmatch(r"(\d{1,3})\.?", word["text"]))
                ),
                key=lambda item: item[1],
            )
            for index, (source_no, anchor_top) in enumerate(anchors):
                if source_no in seen_numbers:
                    raise ValueError(f"duplicate contractor number {source_no}")
                top = header_bottom + 1 if index == 0 else (anchors[index - 1][1] + anchor_top) / 2
                bottom = (
                    (anchor_top + anchors[index + 1][1]) / 2
                    if index + 1 < len(anchors)
                    else min(page.height - 25, anchor_top + 24)
                )
                company = _cell_text(page, (name_x, top, contact_x, bottom))
                contact = _cell_text(page, (contact_x, top, grade_x, bottom))
                prc = _cell_text(page, (grade_x, top, license_x, bottom))
                business = _cell_text(page, (business_x, top, address_x, bottom))
                address = _cell_text(page, (address_x, top, page.width - 20, bottom))
                if not company:
                    raise ValueError(f"contractor {source_no} has no name")
                records.append({
                    "id": source_no,
                    "company": company,
                    "address": address,
                    "business": business,
                    "contact": contact,
                    "prc": prc,
                })
                seen_numbers.add(source_no)

    if not records:
        raise ValueError("PDF parser returned no contractor records")
    records.sort(key=lambda record: record["id"])
    return records


def write_json(path: Path, records: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile("w", encoding="utf-8", dir=path.parent, delete=False) as handle:
        json.dump(records, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
        temporary = Path(handle.name)
    temporary.replace(path)


def post_snapshot(
    session: requests.Session,
    server_url: str,
    token: str,
    source: str,
    records: list[dict[str, Any]],
    timeout: float,
) -> None:
    endpoint = f"{server_url.rstrip('/')}/api/ingest"
    response = session.post(
        endpoint,
        headers={"Authorization": f"Bearer {token}"},
        json={"source": source, "records": records},
        timeout=timeout,
    )
    response.raise_for_status()
    result = response.json()
    logging.info("ingested %s run_id=%s", source, result.get("run_id"))


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--no-post", action="store_true", help="write local JSON only")
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "frontend",
        help="directory for data.json and contractors.json",
    )
    parser.add_argument("--timeout", type=float, default=60.0, help="HTTP timeout in seconds")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    session = requests.Session()
    session.headers["User-Agent"] = USER_AGENT

    barangays = fetch_barangays(session, args.timeout)
    pdf_url, pdf_bytes = fetch_aep_pdf(session, args.timeout)
    contractors = parse_contractors(pdf_bytes)
    logging.info("scraped %d barangay feeder records", len(barangays))
    logging.info("scraped %d contractor records from %s", len(contractors), pdf_url)

    write_json(args.output_dir / "data.json", barangays)
    write_json(args.output_dir / "contractors.json", contractors)
    logging.info("updated JSON snapshots in %s", args.output_dir)

    if args.no_post:
        return
    server_url = os.environ.get("SERVER_URL", "").strip()
    ingest_token = os.environ.get("INGEST_TOKEN", "").strip()
    if not server_url or not ingest_token:
        raise SystemExit("SERVER_URL and INGEST_TOKEN are required unless --no-post is used")
    post_snapshot(session, server_url, ingest_token, SOURCE_BARANGAYS, barangays, args.timeout)
    post_snapshot(session, server_url, ingest_token, SOURCE_CONTRACTORS, contractors, args.timeout)


if __name__ == "__main__":
    main()
