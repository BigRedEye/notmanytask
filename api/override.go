package api

type OverrideRequest struct {
	Token  string `json:"token" form:"token"`
	Task   string `json:"task" form:"task"`
	Login  string `json:"login" form:"login"`
	Score  int    `json:"score" form:"score"`
	Status string `json:"status" form:"status"`
}

type OverrideResponse struct {
	Status
}
