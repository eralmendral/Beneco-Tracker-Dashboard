# BENECO Tracker Dashboard

A static dashboard, scheduled BENECO data scraper, and Go/PostgreSQL history API.
Barangay and contractor ingests create immutable snapshots. Interruption and social-media
complaint records are deduplicated by public source identifier so recurring pulls grow
a durable reliability history without inflating event counts.

## Project layout

- `frontend/` — static dashboard and last-known-good JSON snapshots.
- `scripts/` — Python scraper for barangays, interruption events, the accredited-practitioner PDF, and privacy-minimized Facebook complaints.
- `server/` — Go API, PostgreSQL access, and embedded migrations.
- `.github/workflows/scrape.yml` — daily and manually dispatched scrape job.
- `compose.yaml` — DigitalOcean droplet deployment at
  `/projects/beneco-dashboard/`.

## Run locally

Serve the frontend from the repository root:

```powershell
python -m http.server 8000 --directory frontend
```

Open <http://127.0.0.1:8000/>. When the API is unavailable, the dashboard uses the
bundled `frontend/data.json` and `frontend/contractors.json` snapshots.
When the API is available, operators can use **Pull Data** in the sidebar and
enter the configured ingest token to archive fresh feeder, contractor, unscheduled
interruption, and social-media data in one action. The token is used for that request
only and is not stored by the browser. The **Reliability** workspace ranks recurring
feeder interruptions, maps affected municipalities, and charts unscheduled interruption
frequency. The dedicated **Customer Complaints** workspace analyzes the database archive
with category and time charts, feeder/location coverage, hotspot summaries, and a
searchable list of anonymous public Facebook complaints covering power interruptions,
billing, service, and meter/connection concerns. Contact details are removed before
database storage.

To run the API, copy `server/.env.example` to `server/.env`, set a reachable
PostgreSQL `DATABASE_URL` and a long random `INGEST_TOKEN`, then:

```powershell
cd server
$env:DATABASE_URL = "postgresql://..."
$env:INGEST_TOKEN = "..."
$env:FACEBOOK_ACCESS_TOKEN = "..." # optional; expands collection beyond the featured post
go run ./cmd/server
```

The API listens on `http://127.0.0.1:8080` by default. Database migrations run
automatically at startup.

Refresh the checked-in fallback snapshots without posting to the API:

```powershell
python -m pip install -r scripts/requirements.txt
python scripts/scrape_beneco.py --no-post
```

See [scripts/README.md](scripts/README.md) and [server/README.md](server/README.md)
for component details.

## DigitalOcean droplet deployment

The deployment expects Docker Engine and the Compose plugin on the droplet. It uses
the hosted PostgreSQL database from `DATABASE_URL`; it does not run another database
container.

1. Clone the repository on the droplet.
2. Copy `server/.env.example` to `server/.env` and replace every placeholder.
3. Build and start:

   ```sh
   docker compose up -d --build
   ```

4. Verify:

   ```sh
   curl http://<DROPLET_IP>/projects/beneco-dashboard/api/healthz
   ```

The dashboard is available at
`http://<DROPLET_IP>/projects/beneco-dashboard/`. Set the GitHub Actions
`SERVER_URL` secret to that URL without the trailing slash, and set
`INGEST_TOKEN` to the same value used by the API.
Add the optional `FACEBOOK_ACCESS_TOKEN` secret to collect comments across recent
BENECO page posts through Meta's Pages API; without it, the public post embedded on
BENECO's website remains the fallback source.

Only port 80 is published by Compose. The Go service remains on the private Compose
network, and Nginx proxies the prefixed API path.

### Update and rollback

Before updating, note the currently deployed commit. Pull the desired revision and run
`docker compose up -d --build`. To roll back, check out the prior known-good commit
and run the same command. Database snapshots are insert-only, so application rollback
does not delete scrape history. Configure backups and point-in-time recovery on the
hosted PostgreSQL provider.

## Verification

```powershell
python -m unittest discover -s scripts -p "test_*.py"
python -m compileall -q scripts
cd server
go test ./...
go build ./...
```

The live scraper test is intentionally separate because it depends on BENECO's public
website:

```powershell
python scripts/scrape_beneco.py --no-post
```
