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

func tooLong(value string, limit int) bool {
	return len([]rune(value)) > limit
}
