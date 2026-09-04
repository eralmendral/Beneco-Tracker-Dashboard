package model

import "time"

const (
	SourceBarangayFeeders = "barangay_feeders"
	SourceContractors     = "contractors"
	SourceOutages         = "outages"
	SourceFacebookReports = "facebook_reports"
)

type ScrapeRun struct {
	ID        int64     `json:"id"`
	Source    string    `json:"source"`
	ScrapedAt time.Time `json:"scraped_at"`
}

type BarangayFeeder struct {
	BarangayID   int    `json:"barangayid"`
	Barangay     string `json:"barangay"`
	Municipality string `json:"municipality"`
	Feeder       string `json:"feeder"`
}

type Contractor struct {
	SourceNo *int   `json:"id,omitempty"`
	Company  string `json:"company"`
	Address  string `json:"address"`
	Business string `json:"business"`
	Contact  string `json:"contact"`
	PRC      string `json:"prc"`
}

type OutageEvent struct {
	SourceID        string     `json:"source_id"`
	Feeder          string     `json:"feeder"`
	Area            string     `json:"area"`
	Cause           string     `json:"cause"`
	StartedAt       time.Time  `json:"started_at"`
	RestoredAt      *time.Time `json:"restored_at"`
	DurationMinutes int        `json:"duration_minutes"`
	Status          string     `json:"status"`
	SourceURL       string     `json:"source_url"`
}

type FacebookReport struct {
	SourceID       string    `json:"source_id"`
	PostURL        string    `json:"post_url"`
	ReportedAt     time.Time `json:"reported_at"`
	Location       string    `json:"location"`
	Feeder         string    `json:"feeder"`
	CommentExcerpt string    `json:"comment_excerpt"`
}
