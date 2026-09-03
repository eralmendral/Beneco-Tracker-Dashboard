CREATE TABLE scrape_runs (
  id BIGSERIAL PRIMARY KEY,
  source TEXT NOT NULL CHECK (source IN ('barangay_feeders', 'contractors')),
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

CREATE INDEX barangay_feeders_scrape_run_id_idx
  ON barangay_feeders (scrape_run_id);

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

CREATE INDEX contractors_scrape_run_id_idx
  ON contractors (scrape_run_id);

CREATE INDEX scrape_runs_source_scraped_at_idx
  ON scrape_runs (source, scraped_at DESC, id DESC);
