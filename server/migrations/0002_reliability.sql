ALTER TABLE scrape_runs
  DROP CONSTRAINT IF EXISTS scrape_runs_source_check;

ALTER TABLE scrape_runs
  ADD CONSTRAINT scrape_runs_source_check
  CHECK (source IN ('barangay_feeders', 'contractors', 'outages', 'facebook_reports'));

CREATE TABLE outage_events (
  id BIGSERIAL PRIMARY KEY,
  scrape_run_id BIGINT NOT NULL REFERENCES scrape_runs(id),
  source_id TEXT NOT NULL,
  feeder TEXT NOT NULL,
  area TEXT NOT NULL,
  cause TEXT,
  started_at TIMESTAMPTZ NOT NULL,
  restored_at TIMESTAMPTZ,
  duration_minutes INT NOT NULL DEFAULT 0 CHECK (duration_minutes >= 0),
  status TEXT NOT NULL,
  source_url TEXT NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (source_id, feeder)
);

CREATE INDEX outage_events_started_at_idx
  ON outage_events (started_at DESC);

CREATE INDEX outage_events_feeder_started_at_idx
  ON outage_events (feeder, started_at DESC);

CREATE TABLE facebook_reports (
  id BIGSERIAL PRIMARY KEY,
  scrape_run_id BIGINT NOT NULL REFERENCES scrape_runs(id),
  source_id TEXT NOT NULL,
  post_url TEXT NOT NULL,
  reported_at TIMESTAMPTZ NOT NULL,
  location TEXT,
  feeder TEXT NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (source_id, feeder)
);

CREATE INDEX facebook_reports_reported_at_idx
  ON facebook_reports (reported_at DESC);

CREATE INDEX facebook_reports_feeder_reported_at_idx
  ON facebook_reports (feeder, reported_at DESC);
