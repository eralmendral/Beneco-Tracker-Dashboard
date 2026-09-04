ALTER TABLE facebook_reports
  ADD COLUMN category TEXT NOT NULL DEFAULT 'outage'
  CHECK (category IN ('outage', 'billing', 'service', 'meter_connection'));
