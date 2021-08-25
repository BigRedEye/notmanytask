package api

type Status struct {
	Ok    bool   `json:"Ok"`
	Error string `json:"Error,omitempty"`
}
