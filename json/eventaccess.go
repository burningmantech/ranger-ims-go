package json

type EventsAccess map[string]EventAccess

type AccessRule struct {
	Expression string `json:"expression"`
	Validity   string `json:"validity"`
}

type EventAccess struct {
	Readers   []AccessRule `json:"readers"`
	Writers   []AccessRule `json:"writers"`
	Reporters []AccessRule `json:"reporters"`
}
