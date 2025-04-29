package json

import "time"

type FieldReports []FieldReport
type FieldReport struct {
	Event   string    `json:"event"`
	Number  int32     `json:"number"`
	Created time.Time `json:"created,omitzero"`
	// Summary is nilable, because client can set it to empty, and the server must be able
	// to distinguish empty from unset.
	Summary       *string       `json:"summary"`
	Incident      int32         `json:"incident,omitzero"`
	ReportEntries []ReportEntry `json:"report_entries"`
}
