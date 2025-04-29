package json

type Events []Event
type Event struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}
type EditEventsRequest struct {
	Add []string `json:"add"`
}
