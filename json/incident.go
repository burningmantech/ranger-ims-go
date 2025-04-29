package json

import (
	"time"
)

type Incidents []Incident

type Location struct {
	// Various fields here are nilable, because client can set them empty, and the server must be able
	// to distinguish empty from unset.

	Name         *string `json:"name"`
	Concentric   *string `json:"concentric"`
	RadialHour   *string `json:"radial_hour"`
	RadialMinute *string `json:"radial_minute"`
	Description  *string `json:"description"`
	Type         string  `json:"type"`
}

const (
	IncidentPriorityHigh   = 5
	IncidentPriorityNormal = 3
	IncidentPriorityLow    = 1
)

type Incident struct {
	Event         string        `json:"event"`
	EventID       int32         `json:"event_id"`
	Number        int32         `json:"number"`
	Created       time.Time     `json:"created,omitzero"`
	LastModified  time.Time     `json:"last_modified,omitzero"`
	State         string        `json:"state"`
	Priority      int8          `json:"priority"`
	Summary       *string       `json:"summary"`
	Location      Location      `json:"location"`
	IncidentTypes *[]string     `json:"incident_types"`
	FieldReports  *[]int32      `json:"field_reports"`
	RangerHandles *[]string     `json:"ranger_handles"`
	ReportEntries []ReportEntry `json:"report_entries"`
}
