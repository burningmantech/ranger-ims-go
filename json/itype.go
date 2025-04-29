package json

type IncidentTypes []string

type EditIncidentTypesRequest struct {
	Add  IncidentTypes `json:"add"`
	Hide IncidentTypes `json:"hide"`
	Show IncidentTypes `json:"show"`
}
