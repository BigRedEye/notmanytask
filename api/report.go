package api

// IDs are strings for compatibility
type ReportRequest struct {
	Token       string `json:"token"`
	Task        string `json:"task"`
	UserID      string `json:"user_id"`
	PipelineID  string `json:"pipeline_id"`
	ProjectName string `json:"project_name"`
	Failed      int    `json:"failed,omitempty"`
}

type ReportResponse struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}
