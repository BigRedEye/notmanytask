package api

type Status struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}
