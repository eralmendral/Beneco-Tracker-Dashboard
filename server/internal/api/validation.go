package api

import (
	"errors"
	"fmt"
	"strings"

	"github.com/eralmendral/Beneco-Tracker-Dashboard/server/internal/model"
)

func validateBarangayFeeders(records []model.BarangayFeeder) error {
	if len(records) == 0 {
		return errors.New("records must not be empty")
	}
	if len(records) > maxRecords {
		return fmt.Errorf("records must not contain more than %d items", maxRecords)
	}
	for i := range records {
		record := &records[i]
		record.Barangay = strings.TrimSpace(record.Barangay)
		record.Municipality = strings.TrimSpace(record.Municipality)
		record.Feeder = strings.TrimSpace(record.Feeder)
		if record.BarangayID <= 0 {
			return fmt.Errorf("records[%d].barangayid must be positive", i)
		}
		if record.Barangay == "" || record.Municipality == "" || record.Feeder == "" {
			return fmt.Errorf("records[%d] is missing a required field", i)
		}
		if tooLong(record.Barangay, 500) || tooLong(record.Municipality, 500) || tooLong(record.Feeder, 1000) {
			return fmt.Errorf("records[%d] contains an overlong field", i)
		}
	}
	return nil
}

func validateContractors(records []model.Contractor) error {
	if len(records) == 0 {
		return errors.New("records must not be empty")
	}
	if len(records) > maxRecords {
		return fmt.Errorf("records must not contain more than %d items", maxRecords)
	}
	for i := range records {
		record := &records[i]
		record.Company = strings.TrimSpace(record.Company)
		record.Address = strings.TrimSpace(record.Address)
		record.Business = strings.TrimSpace(record.Business)
		record.Contact = strings.TrimSpace(record.Contact)
		record.PRC = strings.TrimSpace(record.PRC)
		if record.SourceNo != nil && *record.SourceNo <= 0 {
			return fmt.Errorf("records[%d].id must be positive", i)
		}
		if record.Company == "" {
			return fmt.Errorf("records[%d].company is required", i)
		}
		if tooLong(record.Company, 500) || tooLong(record.Address, 2000) ||
			tooLong(record.Business, 1000) || tooLong(record.Contact, 500) || tooLong(record.PRC, 100) {
			return fmt.Errorf("records[%d] contains an overlong field", i)
		}
	}
	return nil
}

func validateOutages(records []model.OutageEvent) error {
	if len(records) == 0 {
		return errors.New("records must not be empty")
	}
	if len(records) > maxRecords {
		return fmt.Errorf("records must not contain more than %d items", maxRecords)
	}
	for i := range records {
		record := &records[i]
		record.SourceID = strings.TrimSpace(record.SourceID)
		record.Feeder = strings.TrimSpace(record.Feeder)
		record.Area = strings.TrimSpace(record.Area)
		record.Cause = strings.TrimSpace(record.Cause)
		record.Status = strings.TrimSpace(record.Status)
		record.SourceURL = strings.TrimSpace(record.SourceURL)
		if record.SourceID == "" || record.Feeder == "" || record.Area == "" ||
			record.Status == "" || record.SourceURL == "" || record.StartedAt.IsZero() {
			return fmt.Errorf("records[%d] is missing a required field", i)
		}
		if record.DurationMinutes < 0 {
			return fmt.Errorf("records[%d].duration_minutes must not be negative", i)
		}
		if tooLong(record.SourceID, 100) || tooLong(record.Feeder, 100) ||
			tooLong(record.Area, 10_000) || tooLong(record.Cause, 2000) ||
			tooLong(record.Status, 100) || tooLong(record.SourceURL, 2000) {
			return fmt.Errorf("records[%d] contains an overlong field", i)
		}
	}
	return nil
}

func validateFacebookReports(records []model.FacebookReport) error {
	if len(records) == 0 {
		return errors.New("records must not be empty")
	}
	if len(records) > maxRecords {
		return fmt.Errorf("records must not contain more than %d items", maxRecords)
	}
	for i := range records {
		record := &records[i]
		record.SourceID = strings.TrimSpace(record.SourceID)
		record.PostURL = strings.TrimSpace(record.PostURL)
		record.Location = strings.TrimSpace(record.Location)
		record.Feeder = strings.TrimSpace(record.Feeder)
		record.CommentExcerpt = strings.TrimSpace(record.CommentExcerpt)
		if record.SourceID == "" || record.PostURL == "" || record.Feeder == "" || record.ReportedAt.IsZero() {
			return fmt.Errorf("records[%d] is missing a required field", i)
		}
		if tooLong(record.SourceID, 200) || tooLong(record.PostURL, 2000) ||
			tooLong(record.Location, 500) || tooLong(record.Feeder, 100) ||
			tooLong(record.CommentExcerpt, 280) {
			return fmt.Errorf("records[%d] contains an overlong field", i)
		}
	}
	return nil
}

func tooLong(value string, limit int) bool {
	return len([]rune(value)) > limit
}
