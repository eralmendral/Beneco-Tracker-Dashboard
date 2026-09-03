package model

import "time"

const (
	SourceBarangayFeeders = "barangay_feeders"
	SourceContractors     = "contractors"
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
