package json

import "time"

type ReportEntry struct {
	ID            int32     `json:"id"`
	Created       time.Time `json:"created,omitzero"`
	Author        string    `json:"author"`
	SystemEntry   bool      `json:"system_entry"`
	Text          string    `json:"text"`
	Stricken      bool      `json:"stricken"`
	HasAttachment bool      `json:"has_attachment"`
}
