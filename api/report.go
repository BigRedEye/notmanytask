package api

// IDs are strings for compatibility
type ReportRequest struct {
	Token       string `json:"token" form:"token"`
	Task        string `json:"task" form:"task"`
	UserID      string `json:"user_id" form:"user_id"`
	PipelineID  string `json:"pipeline_id" form:"pipeline_id"`
	ProjectName string `json:"project_name" form:"project_name"`
	Failed      int    `json:"failed,omitempty" form:"failed"`
	Status      string `json:"status,omitempty" form:"status"`
}

type ReportResponse struct {
	Status
}
