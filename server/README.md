# BENECO history API

The Go service owns PostgreSQL access and audits every scrape as a new run. Barangay
and contractor records are immutable snapshots; interruption and Facebook signal
records are deduplicated by public source identifier. Each ingest remains transactional.

## Configuration

- `DATABASE_URL` — PostgreSQL connection URL (required).
- `INGEST_TOKEN` — long random bearer token used by ingest and manual scrape requests (required).
- `PORT` — HTTP port; defaults to `8080`.
- `SCRAPER_PYTHON` — optional Python executable override.
- `SCRAPER_SCRIPT` — optional path to `scrape_beneco.py`.
- `SCRAPER_OUTPUT_DIR` — optional refreshed-JSON destination; defaults to a temporary directory.
- `FACEBOOK_ACCESS_TOKEN` — optional server-side Meta token that expands complaint
  collection from BENECO's featured public post to recent page posts and comments.

Copy `.env.example` to `.env` for Docker Compose. The Go process reads environment
variables directly and does not load dotenv files itself.

## API

- `GET /api/healthz` — process and database health.
- `POST /api/ingest` — authenticated snapshot ingest.
- `POST /api/scrape` — authenticated live BENECO scrape and ingest. Only one can run at a time.
- `GET /api/scrape-runs?source=barangay_feeders|contractors|outages|facebook_reports` — run history.
- `GET /api/barangay-feeders/latest` — latest barangay snapshot.
- `GET /api/contractors/latest` — latest contractor snapshot.
- `GET /api/barangay-feeders?run_id=<id>` — historical barangay snapshot.
- `GET /api/contractors?run_id=<id>` — historical contractor snapshot.
- `GET /api/outages?days=90` — deduplicated official interruption events from the last 7–365 days.
- `GET /api/facebook-reports?days=90` — privacy-minimized public Facebook outage signals from the last 7–365 days.

Example ingest:

```sh
curl -X POST http://127.0.0.1:8080/api/ingest \
  -H "Authorization: Bearer $INGEST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source":"barangay_feeders","records":[{"barangayid":1,"barangay":"Abiang","municipality":"ATOK","feeder":"CIRCUIT_02"}]}'
```

Public GET responses allow cross-origin reads. Browser-originated POST requests are
not granted CORS access, and the token must never be shipped in frontend code.
The dashboard's **Pull Data** button asks the operator for the token at action time
and does not store it. A scrape request stays open until the current service-area,
contractor, unscheduled-interruption, and public social datasets have been archived or
the two-minute server timeout is reached. Facebook signal collection is best-effort so
a markup or access change on Facebook does not block the official BENECO datasets.

Interruption events and Facebook reports are deduplicated by their public source
identifiers. Facebook records contain the reported time, inferred location and feeder,
source post URL, and a short anonymous complaint excerpt. Profile names are discarded,
and common contact details are redacted before ingest. Unmapped complaints are retained
for the comments view but excluded from feeder scoring.

## Development

```powershell
python -m pip install -r ..\scripts\requirements.txt
go test ./...
go build ./...
go run ./cmd/server
```

The Docker image includes Python, the scraper, and its pinned requirements. For a
direct local Go run, install the Python requirements first as shown above.

Migrations are embedded in the binary and tracked in `schema_migrations`. This
repository has no local PostgreSQL container; database integration verification
requires a supplied `DATABASE_URL`.
