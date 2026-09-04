# BENECO scraper

`scrape_beneco.py` downloads the public barangay-to-feeder JSON and official BENECO
interruption feed, discovers the current accredited electrical practitioners PDF,
and parses its table by row and column coordinates. It also inspects BENECO's featured
Facebook post plus recent posts exposed by Facebook's public Page Plugin. Recognized
English, Tagalog, and common Ilocano power-interruption phrases, along with billing,
customer-service, and meter/connection complaints, are stored. When
`FACEBOOK_ACCESS_TOKEN` is configured, the same pull additionally uses
Meta's Pages API to inspect up to 30 recent BENECO page posts and up to 300 comments
per post.

## Setup and local use

Run from the repository root:

```powershell
python -m venv .venv
.\.venv\Scripts\Activate.ps1
python -m pip install -r scripts/requirements.txt
python scripts/scrape_beneco.py --no-post
```

`--no-post` refreshes `frontend/data.json` and
`frontend/contractors.json` only. Use `--output-dir <path>` for a different
destination.

To create immutable server snapshots, omit `--no-post` and set:

- `SERVER_URL` — API base URL, such as
  `http://<DROPLET_IP>/projects/beneco-dashboard`.
- `INGEST_TOKEN` — bearer token matching the server configuration.
- `FACEBOOK_ACCESS_TOKEN` — optional server-side Meta access token with permission
  to read the BENECO page's public posts and comments. Never expose this token in the
  browser. Use a token issued through a Meta Developer app; do not store a personal
  Facebook password or browser cookies in this project.

The script fails instead of posting an empty required dataset. Facebook parsing is
best-effort because public embed markup and API access may change. It retains the
comment identifier, time, inferred location and feeder, source post URL, and an
anonymous excerpt of at most 240 characters. Profile names are excluded and common
phone numbers, email addresses, account-like numbers, links, and mentions are redacted.
Complaints are categorized as `outage`, `billing`, `service`, or `meter_connection`.
Those that cannot be mapped to a feeder are retained as `UNMAPPED`; only feeder-matched
outage complaints affect reliability scores. Logs include record counts so
source-layout changes are visible in GitHub Actions.

## Tests

```powershell
python -m unittest discover -s scripts -p "test_*.py"
python -m compileall -q scripts
```
