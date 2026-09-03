# BENECO scraper

`scrape_beneco.py` downloads the public barangay-to-feeder JSON, discovers the
current accredited electrical practitioners PDF from BENECO's forms page, and parses
its table by row and column coordinates.

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

The script fails instead of posting an empty dataset. Logs include the source URL and
record counts so source-layout changes are visible in GitHub Actions.

## Tests

```powershell
python -m unittest discover -s scripts -p "test_*.py"
python -m compileall -q scripts
```
