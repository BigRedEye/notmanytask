package api

type FlagRequest struct {
	Token string `json:"token" form:"token"`
	Task  string `json:"task" form:"task"`
}

type FlagResponse struct {
	Status
	Flag string `json:"flag,omitempty" form:"flag"`
}
