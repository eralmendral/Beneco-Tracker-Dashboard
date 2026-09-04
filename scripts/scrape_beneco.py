#!/usr/bin/env python3
"""Scrape BENECO's public service datasets and ingest durable history."""

from __future__ import annotations

import argparse
import datetime as dt
import html
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
BENECO_HOME_URL = "https://beneco.com.ph/index.html"
OUTAGE_URL = "https://api.beneco.com.ph/wballoutages.php"
FACEBOOK_PAGE_URL = "https://www.facebook.com/benguetelectric"
FACEBOOK_GRAPH_URL = "https://graph.facebook.com"
FACEBOOK_PAGE_ID = "793198510733155"
SOURCE_BARANGAYS = "barangay_feeders"
SOURCE_CONTRACTORS = "contractors"
SOURCE_OUTAGES = "outages"
SOURCE_FACEBOOK_REPORTS = "facebook_reports"
USER_AGENT = "Beneco-Tracker-Dashboard/1.0 (+scheduled public-data archive)"
MANILA_TIMEZONE = dt.timezone(dt.timedelta(hours=8))
OUTAGE_COMPLAINT_PATTERN = re.compile(
    r"(?:"
    r"wala(?:ng|\s+pa(?:ng|\s+rin)?|\s+parin)?\s+(?:kuryente|ilaw)|"
    r"(?:kuryente|ilaw)\s+(?:wala|patay|off)|"
    r"brown\s*out|black\s*out|"
    r"no\s+(?:power|electricity)|power\s+(?:is\s+out|outage|interruption)|"
    r"unscheduled\s+(?:power\s+)?interruption"
    r")",
    flags=re.IGNORECASE,
)


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


def normalize_feeders(value: str) -> list[str]:
    normalized = " ".join(str(value).upper().split())
    feeders: list[str] = []
    label = re.search(r"\b(FEEDER|CIRCUIT)S?\s*[_-]?\s*", normalized)
    if label:
        sequence = re.match(
            r"\d{1,2}[A-Z]?(?:(?:\s*[,/&-]\s*|\s+)\d{1,2}[A-Z]?)*",
            normalized[label.end():],
        )
        if sequence:
            prefix = "FEEDER" if label.group(1) == "FEEDER" else "CIRCUIT"
            feeders.extend(
                f"{prefix}_{int(number):02d}{suffix}"
                for number, suffix in re.findall(r"(\d{1,2})([A-Z]?)", sequence.group(0))
            )

    for name in ("DALICNO", "LUELCO", "TAPSAN"):
        if name in normalized:
            feeders.append(f"FEEDER_{name}")
    if normalized == "NGCP":
        feeders.append("NGCP")
    elif "HEDCOR" in normalized:
        feeders.append("HEDCOR_BAKUN")

    return list(dict.fromkeys(feeders))


def _beneco_timestamp(value: Any) -> dt.datetime | None:
    text = str(value or "").strip()
    if not text or text.startswith("0000-00-00"):
        return None
    try:
        return dt.datetime.strptime(text, "%Y-%m-%d %H:%M:%S").replace(tzinfo=MANILA_TIMEZONE)
    except ValueError:
        return None


def _clean_area(value: Any) -> str:
    text = re.sub(r"<br\s*/?>", "\n", str(value or ""), flags=re.IGNORECASE)
    text = re.sub(r"<[^>]+>", " ", text)
    text = html.unescape(html.unescape(text))
    return "\n".join(" ".join(line.split()) for line in text.splitlines() if line.strip())


def fetch_outages(session: requests.Session, timeout: float) -> list[dict[str, Any]]:
    source_rows: dict[str, dict[str, Any]] = {}
    for period in ("last_week", "this_week"):
        response = session.get(OUTAGE_URL, params={"period": period}, timeout=timeout)
        response.raise_for_status()
        payload = response.json()
        if not isinstance(payload, list):
            raise ValueError(f"outage endpoint returned an unexpected {period} response")
        for item in payload:
            if isinstance(item, dict) and item.get("id") is not None:
                source_rows[str(item["id"])] = item

    records: list[dict[str, Any]] = []
    for source_id, item in source_rows.items():
        started_at = _beneco_timestamp(item.get("timeoff"))
        area = _clean_area(item.get("area"))
        feeders = normalize_feeders(str(item.get("feeder", "")))
        if started_at is None or not area or not feeders:
            logging.warning("skipping incomplete outage id=%s", source_id)
            continue
        restored_at = _beneco_timestamp(item.get("timerestored"))
        duration_minutes = 0
        if restored_at is not None and restored_at >= started_at:
            duration_minutes = int((restored_at - started_at).total_seconds() // 60)
        for feeder in feeders:
            records.append({
                "source_id": source_id,
                "feeder": feeder,
                "area": area,
                "cause": _clean_area(item.get("cause")),
                "started_at": started_at.isoformat(),
                "restored_at": restored_at.isoformat() if restored_at else None,
                "duration_minutes": duration_minutes,
                "status": " ".join(str(item.get("status") or "Unknown").split()),
                "source_url": BENECO_HOME_URL,
            })
    if not records:
        raise ValueError("outage endpoint returned no usable records")
    records.sort(key=lambda record: (record["started_at"], record["source_id"], record["feeder"]))
    return records


def discover_facebook_embed_url(home_html: str) -> str:
    match = re.search(
        r'<iframe[^>]+src=["\']([^"\']*facebook\.com/plugins/post\.php[^"\']*)["\']',
        home_html,
        flags=re.IGNORECASE,
    )
    if not match:
        raise ValueError("could not find BENECO's embedded Facebook post")
    return html.unescape(match.group(1))


def _normalize_location(value: str) -> str:
    return re.sub(r"[^A-Z0-9]+", " ", value.upper()).strip()


def _report_location(text: str, barangays: list[dict[str, Any]]) -> str:
    compact = " ".join(text.split())
    boundary = re.search(
        r"\b(?:since|simula|mula|walang\s+kuryente|brownout|blackout|no\s+power|power\s+outage)\b",
        compact,
        flags=re.IGNORECASE,
    )
    candidate = compact[:boundary.start()] if boundary else ""
    candidate = re.sub(r"^(?:dito\s+)?sa\s+", "", candidate, flags=re.IGNORECASE).strip(" ,;:-")
    if 2 <= len(candidate) <= 100:
        return candidate

    normalized = _normalize_location(compact)
    matches = [
        str(record["barangay"])
        for record in barangays
        if len(_normalize_location(str(record["barangay"]))) >= 4
        and _normalize_location(str(record["barangay"])) in normalized
    ]
    return max(matches, key=len, default="")


def _is_outage_complaint(text: str) -> bool:
    return OUTAGE_COMPLAINT_PATTERN.search(" ".join(text.split())) is not None


def _comment_excerpt(text: str, limit: int = 240) -> str:
    """Keep a useful anonymous excerpt while removing common contact identifiers."""
    excerpt = " ".join(text.split())
    excerpt = re.sub(r"https?://\S+", "[link removed]", excerpt, flags=re.IGNORECASE)
    excerpt = re.sub(r"\b[\w.+-]+@[\w.-]+\.[A-Za-z]{2,}\b", "[email removed]", excerpt)
    excerpt = re.sub(r"(?<!\w)@\w{2,}", "[name removed]", excerpt)
    excerpt = re.sub(r"(?<!\d)(?:\+?63|0)?\d(?:[\s-]?\d){6,}(?!\d)", "[number removed]", excerpt)
    if len(excerpt) <= limit:
        return excerpt
    return excerpt[:limit - 1].rstrip() + "…"


def _report_feeders(
    text: str,
    location: str,
    outages: list[dict[str, Any]],
    barangays: list[dict[str, Any]],
) -> list[str]:
    direct = normalize_feeders(text)
    if direct:
        return direct

    normalized_text = _normalize_location(text)
    normalized_location = _normalize_location(location)
    ignored = {"BARANGAY", "CITY", "SUBDIVISION", "VILLAGE", "PUROK", "PARTS", "PROPER"}
    location_tokens = [
        token for token in normalized_location.split()
        if len(token) >= 4 and token not in ignored
    ]
    matched: list[str] = []
    if location_tokens:
        for outage in outages:
            normalized_area = _normalize_location(str(outage["area"]))
            if all(token in normalized_area for token in location_tokens):
                matched.append(str(outage["feeder"]))
    if matched:
        return list(dict.fromkeys(matched))

    for record in barangays:
        barangay = _normalize_location(str(record["barangay"]))
        if len(barangay) >= 4 and barangay in normalized_text:
            matched.extend(part.strip() for part in str(record["feeder"]).split(",") if part.strip())
    return list(dict.fromkeys(matched))


def parse_facebook_reports(
    embed_html: str,
    outages: list[dict[str, Any]],
    barangays: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    reports: list[dict[str, Any]] = []
    seen: set[tuple[str, str]] = set()
    scripts = re.findall(
        r'<script[^>]+type=["\']application/ld\+json["\'][^>]*>(.*?)</script>',
        embed_html,
        flags=re.IGNORECASE | re.DOTALL,
    )
    for script in scripts:
        try:
            post = json.loads(html.unescape(script))
        except (TypeError, json.JSONDecodeError):
            continue
        comments = post.get("comment", []) if isinstance(post, dict) else []
        if isinstance(comments, dict):
            comments = [comments]
        for comment in comments:
            if not isinstance(comment, dict):
                continue
            text = " ".join(str(comment.get("text") or "").split())
            if not text or not _is_outage_complaint(text):
                continue
            source_id = str(comment.get("identifier") or "").strip()
            reported_text = str(comment.get("dateCreated") or "").strip()
            try:
                reported_at = dt.datetime.fromisoformat(reported_text)
            except ValueError:
                continue
            location = _report_location(text, barangays)
            feeders = _report_feeders(text, location, outages, barangays) or ["UNMAPPED"]
            for feeder in feeders:
                key = (source_id, feeder)
                if not source_id or key in seen:
                    continue
                seen.add(key)
                reports.append({
                    "source_id": source_id,
                    "post_url": str(post.get("url") or FACEBOOK_PAGE_URL),
                    "reported_at": reported_at.isoformat(),
                    "location": location,
                    "feeder": feeder,
                    "comment_excerpt": _comment_excerpt(text),
                })
    reports.sort(key=lambda record: (record["reported_at"], record["source_id"], record["feeder"]))
    return reports


def parse_graph_facebook_reports(
    payload: dict[str, Any],
    outages: list[dict[str, Any]],
    barangays: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    """Normalize comments returned by Meta's supported Pages API."""
    posts = payload.get("data", []) if isinstance(payload, dict) else []
    reports: list[dict[str, Any]] = []
    seen: set[tuple[str, str]] = set()
    for post in posts:
        if not isinstance(post, dict):
            continue
        post_url = str(post.get("permalink_url") or FACEBOOK_PAGE_URL)
        comment_block = post.get("comments", {})
        comments = comment_block.get("data", []) if isinstance(comment_block, dict) else []
        for comment in comments:
            if not isinstance(comment, dict):
                continue
            text = " ".join(str(comment.get("message") or "").split())
            if not text or not _is_outage_complaint(text):
                continue
            source_id = str(comment.get("id") or "").strip()
            try:
                reported_at = dt.datetime.fromisoformat(str(comment.get("created_time") or ""))
            except ValueError:
                continue
            location = _report_location(text, barangays)
            feeders = _report_feeders(text, location, outages, barangays) or ["UNMAPPED"]
            for feeder in feeders:
                key = (source_id, feeder)
                if not source_id or key in seen:
                    continue
                seen.add(key)
                reports.append({
                    "source_id": source_id,
                    "post_url": post_url,
                    "reported_at": reported_at.isoformat(),
                    "location": location,
                    "feeder": feeder,
                    "comment_excerpt": _comment_excerpt(text),
                })
    reports.sort(key=lambda record: (record["reported_at"], record["source_id"], record["feeder"]))
    return reports


def fetch_graph_facebook_reports(
    session: requests.Session,
    access_token: str,
    outages: list[dict[str, Any]],
    barangays: list[dict[str, Any]],
    timeout: float,
) -> list[dict[str, Any]]:
    """Fetch up to 30 recent page posts and their first 300 comments per post."""
    url = f"{FACEBOOK_GRAPH_URL}/{FACEBOOK_PAGE_ID}/posts"
    params: dict[str, Any] | None = {
        "access_token": access_token,
        "limit": 10,
        "fields": "id,permalink_url,created_time,comments.limit(100){id,message,created_time}",
    }
    posts: list[dict[str, Any]] = []
    for _ in range(3):
        response = session.get(url, params=params, timeout=timeout)
        response.raise_for_status()
        payload = response.json()
        if not isinstance(payload, dict):
            raise ValueError("Facebook Pages API returned an unexpected response")
        page_posts = payload.get("data", [])
        if not isinstance(page_posts, list):
            raise ValueError("Facebook Pages API returned invalid post data")
        for post in page_posts:
            if not isinstance(post, dict):
                continue
            comment_block = post.get("comments", {})
            if isinstance(comment_block, dict):
                comments = list(comment_block.get("data", []))
                next_comments = comment_block.get("paging", {}).get("next")
                for _ in range(2):
                    if not next_comments:
                        break
                    comment_response = session.get(next_comments, timeout=timeout)
                    comment_response.raise_for_status()
                    comment_payload = comment_response.json()
                    comments.extend(comment_payload.get("data", []))
                    next_comments = comment_payload.get("paging", {}).get("next")
                post["comments"] = {"data": comments}
            posts.append(post)
        next_posts = payload.get("paging", {}).get("next")
        if not next_posts:
            break
        url, params = str(next_posts), None
    return parse_graph_facebook_reports({"data": posts}, outages, barangays)


def fetch_facebook_reports(
    session: requests.Session,
    outages: list[dict[str, Any]],
    barangays: list[dict[str, Any]],
    timeout: float,
) -> list[dict[str, Any]]:
    reports: list[dict[str, Any]] = []
    try:
        home_response = session.get(BENECO_HOME_URL, timeout=timeout)
        home_response.raise_for_status()
        embed_url = discover_facebook_embed_url(home_response.text)
        embed_response = session.get(embed_url, timeout=timeout)
        embed_response.raise_for_status()
        reports.extend(parse_facebook_reports(embed_response.text, outages, barangays))
    except (requests.RequestException, ValueError):
        logging.warning("could not read BENECO's featured Facebook post")

    access_token = os.getenv("FACEBOOK_ACCESS_TOKEN", "").strip()
    if access_token:
        try:
            reports.extend(fetch_graph_facebook_reports(
                session, access_token, outages, barangays, timeout,
            ))
        except (requests.RequestException, ValueError, TypeError):
            logging.warning("Facebook Pages API collection failed; using public embed data only")

    unique = {
        (record["source_id"], record["feeder"]): record
        for record in reports
    }
    return sorted(
        unique.values(),
        key=lambda record: (record["reported_at"], record["source_id"], record["feeder"]),
    )


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
    outages = fetch_outages(session, args.timeout)
    try:
        facebook_reports = fetch_facebook_reports(session, outages, barangays, args.timeout)
    except (requests.RequestException, ValueError) as error:
        logging.warning("Facebook community signals unavailable: %s", error)
        facebook_reports = []

    logging.info("scraped %d barangay feeder records", len(barangays))
    logging.info("scraped %d contractor records from %s", len(contractors), pdf_url)
    logging.info("scraped %d official interruption records", len(outages))
    logging.info("matched %d privacy-minimized Facebook outage reports", len(facebook_reports))

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
    post_snapshot(session, server_url, ingest_token, SOURCE_OUTAGES, outages, args.timeout)
    if facebook_reports:
        post_snapshot(
            session,
            server_url,
            ingest_token,
            SOURCE_FACEBOOK_REPORTS,
            facebook_reports,
            args.timeout,
        )


if __name__ == "__main__":
    main()
