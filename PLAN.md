# BENECO Dashboard: restructure + scraper + history server

## Context

The repo is currently a flat set of static files (`index.html`, `contractor-map.html`,
`data.json`, `contractors.json`, icons, manifest/service worker) with no git history and
no backend. `data.json`/`contractors.json` were manually captured snapshots of two BENECO
sources and are already going stale.

The user wants to:
1. Be able to re-scrape BENECO's live data on a schedule instead of manually.
2. Split the project into `frontend` / `scripts` / `server` so each concern is independent.
3. Persist every scrape as an **immutable historical record** (insert-only) in a Postgres
   database, so the dashboard can show "current" data and later do year-over-year
   comparisons, and so the project retains a copy even if BENECO ever takes its data down.
4. Push the result to `git@github.com:eralmendral/Beneco-Tracker-Dashboard.git`.

Confirmed stack decisions (from the user): **server in Go**, **Postgres** (hosted, via
`DATABASE_URL`), **CI scraper POSTs to a server ingest API** (the server owns the DB, the
scraper never touches it directly).

## Data sources (verified live, today)

- **Barangay → Feeder**: `GET https://api.beneco.com.ph/cwp/barangay.php` returns a flat
  JSON array `{barangayid, barangay, municipality, feeder}` — confirmed byte-for-byte
  match with the current `data.json`. This is what `feeders.php` on the main site calls
  via a DataTables `ajax` config. No auth, no pagination.
- **Accredited contractors (AEP list)**: not a JSON API — it's a PDF linked from
  `https://beneco.com.ph/forms.php` ("List of Accredited Electrical Practitioners"),
  currently `forms/BENECO_Accredited_Electrical_Practitioners_AEP_ 20240319.pdf`. The
  filename/date changes each time BENECO updates it, so the scraper must **discover the
  current link each run** by re-parsing `forms.php`, not hardcode the filename.
  - `pdftotext -layout` output is unreliable (multi-line wrapped rows, misaligned
    columns) — confirmed by inspection. `pdfplumber` (pip-installed and tested available
    in this environment) must be used instead, reconstructing rows via word x/y
    coordinates keyed on the leading numeric "No." column, then assigning columns by
    x-band to match the header (`NAME`, `CONTACT NUMBER`, `GRADE`, `LICENSE NO.`,
    `BUSINESS TRADE NAME`, `BUSINESS ADDRESS`). Target shape matches today's
    `contractors.json` (76 rows): `{company, address, business, contact, prc}`.
  - No system dependency on `poppler`/`pdftotext` needed for the real implementation —
    `pdfplumber` is pure-Python-usable and works the same in CI.

## Frontend data wiring (must change)

- `contractor-map.html` already `fetch()`s `contractors.json` and `data.json` at runtime
  — no change needed there besides the file move.
- `index.html` currently has the **entire barangay/feeder dataset hardcoded** as a JS
  `const RAW = [...]` literal (~line 851), not fetched. For scraped/refreshed data to
  ever reach the main dashboard, `index.html` must be changed to `fetch('data.json')` at
  load time (mirroring `contractor-map.html`'s pattern) instead of embedding the array.
  This is required to make the rest of this plan actually do anything visible on the
  main page, not optional polish.
- Per explicit user instruction this turn, **no Vercel config changes** — `.vercel/` is
  left untouched.

## Target repo layout

```
/frontend
  index.html            (moved; RAW array replaced with fetch('data.json'))
  contractor-map.html   (moved, unchanged)
  manifest.webmanifest, sw.js, icon-192.*, icon-512.*, beneco-logo.png  (moved)
  data.json, contractors.json   (moved; last-known-good snapshot / offline fallback,
                                  overwritten locally each time scripts/scrape_beneco.py
                                  runs, but NOT auto-committed by CI — see below)

/scripts
  scrape_beneco.py       (fetch barangay API + discover/parse AEP PDF; write local
                          frontend/*.json; POST both datasets to the server)
  requirements.txt        (requests, pdfplumber)
  README.md               (env vars: SERVER_URL, INGEST_TOKEN; local run instructions)

/server                   (Go)
  cmd/server/main.go
  internal/db/            (pgx connection, migrations runner)
  internal/api/            (handlers below)
  migrations/0001_init.sql
  go.mod
  .env.example             (DATABASE_URL, INGEST_TOKEN, PORT)
  README.md

/.github/workflows/scrape.yml   (schedule + workflow_dispatch; runs the scraper against
                                 the deployed server; secrets: SERVER_URL, INGEST_TOKEN)

README.md (root, links the three pieces together)
.gitignore (extended: __pycache__/, *.pyc, .venv/, server build output, .env)
```

## Server: append-only history design

```sql
CREATE TABLE scrape_runs (
  id BIGSERIAL PRIMARY KEY,
  source TEXT NOT NULL,               -- 'barangay_feeders' | 'contractors'
  scraped_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE barangay_feeders (
  id BIGSERIAL PRIMARY KEY,
  scrape_run_id BIGINT NOT NULL REFERENCES scrape_runs(id),
  barangayid INT NOT NULL,
  barangay TEXT NOT NULL,
  municipality TEXT NOT NULL,
  feeder TEXT NOT NULL
);

CREATE TABLE contractors (
  id BIGSERIAL PRIMARY KEY,
  scrape_run_id BIGINT NOT NULL REFERENCES scrape_runs(id),
  source_no INT,
  company TEXT NOT NULL,
  address TEXT,
  business TEXT,
  contact TEXT,
  prc TEXT
);
```

Every ingest creates a new `scrape_runs` row and inserts its records tagged with that
run id, inside one transaction. Nothing is ever updated or deleted — "current" is just
"rows from the newest `scrape_runs` per source", and year-over-year comparison is a
query across runs grouped by year.

API (stdlib `net/http`, Go 1.22+ pattern routing — no external router dependency; one
external module: `jackc/pgx/v5` for Postgres):
- `POST /api/ingest` — `Authorization: Bearer <INGEST_TOKEN>` required. Body
  `{"source": "...", "records": [...]}`. Creates a run, batch-inserts records.
- `GET /api/barangay-feeders/latest`, `GET /api/contractors/latest` — newest snapshot.
- `GET /api/scrape-runs?source=` — run ids + timestamps, for building comparison UIs.
- `GET /api/barangay-feeders?run_id=`, `GET /api/contractors?run_id=` — a specific
  historical snapshot.
- `GET /api/healthz`.
- CORS allowing GET from the frontend's origin; POST is server-to-server only (CI → server).

## Scraper script behavior

1. `GET api.beneco.com.ph/cwp/barangay.php` → list of barangay/feeder records.
2. `GET beneco.com.ph/forms.php` → regex out the current AEP PDF link → download it.
3. Parse the PDF table with `pdfplumber` into contractor records (see above).
4. Write both datasets to `frontend/data.json` / `frontend/contractors.json` (pretty
   JSON) as a local cache/fallback.
5. `POST` both datasets to `${SERVER_URL}/api/ingest` with the bearer token.
6. Log record counts for both sources (cheap sanity signal if BENECO's page structure
   changes and the parse silently comes back empty/short) — no complex anomaly
   detection, just visible counts in the run log.

## CI

`.github/workflows/scrape.yml`: `schedule` (daily cron) + `workflow_dispatch`. Steps:
checkout → setup-python → `pip install -r scripts/requirements.txt` → run
`scripts/scrape_beneco.py` with `SERVER_URL`/`INGEST_TOKEN` repo secrets. CI does **not**
commit the refreshed JSON back to the repo (keeps the workflow simple, no bot-commit/PAT
plumbing) — `frontend/*.json` stays as the last manually-committed snapshot; the
long-term path for the frontend to see fresh data is reading the server's `latest`
endpoints, which is a natural but separate follow-up not built in this pass.

## Git / GitHub

- `git init`, extend `.gitignore`, stage everything, initial commit.
- `git remote add origin git@github.com:eralmendral/Beneco-Tracker-Dashboard.git`.
- SSH auth to GitHub as `eralmendral` already verified working in this environment.
- `git push -u origin main`.

## Verification

- Frontend: already smoke-tested this session (`python3 -m http.server` from
  `/frontend`, confirmed `contractors.json`/`data.json` fetch correctly); re-run after
  the `index.html` RAW→fetch change and confirm the dashboard still renders identically.
- Scraper: run locally with a `--no-post` flag, compare output counts against the
  existing 76-row `contractors.json` and the barangay list; inspect a handful of parsed
  contractor rows by eye against the source PDF for column-alignment correctness.
- Server: `go build ./...`; **no local Postgres is available in this environment**, so
  end-to-end ingest/read testing needs a `DATABASE_URL` the user supplies (e.g. a free
  Neon/Supabase instance) — run migrations, `POST /api/ingest` with a small sample
  payload twice, confirm `GET .../latest` reflects only the second run while
  `GET /api/scrape-runs` shows both, proving history is preserved.
- Git: `git log`, `git remote -v`, `git ls-remote origin` after push to confirm it landed.
