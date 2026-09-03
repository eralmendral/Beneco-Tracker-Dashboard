# BENECO history API

The Go service owns PostgreSQL access and stores every scrape as a new run. Ingest is
transactional: either the run and all records are committed, or none are.

## Configuration

- `DATABASE_URL` — PostgreSQL connection URL (required).
- `INGEST_TOKEN` — long random bearer token used only by the scraper (required).
- `PORT` — HTTP port; defaults to `8080`.

Copy `.env.example` to `.env` for Docker Compose. The Go process reads environment
variables directly and does not load dotenv files itself.

## API

- `GET /api/healthz` — process and database health.
- `POST /api/ingest` — authenticated snapshot ingest.
- `GET /api/scrape-runs?source=barangay_feeders|contractors` — run history.
- `GET /api/barangay-feeders/latest` — latest barangay snapshot.
- `GET /api/contractors/latest` — latest contractor snapshot.
- `GET /api/barangay-feeders?run_id=<id>` — historical barangay snapshot.
- `GET /api/contractors?run_id=<id>` — historical contractor snapshot.

Example ingest:

```sh
curl -X POST http://127.0.0.1:8080/api/ingest \
  -H "Authorization: Bearer $INGEST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source":"barangay_feeders","records":[{"barangayid":1,"barangay":"Abiang","municipality":"ATOK","feeder":"CIRCUIT_02"}]}'
```

Public GET responses allow cross-origin reads. Browser-originated POST requests are
not granted CORS access, and the token must never be shipped in frontend code.

## Development

```powershell
go test ./...
go build ./...
go run ./cmd/server
```

Migrations are embedded in the binary and tracked in `schema_migrations`. This
repository has no local PostgreSQL container; database integration verification
requires a supplied `DATABASE_URL`.
