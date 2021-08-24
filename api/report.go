package api

import "github.com/bigredeye/notmanytask/internal/scorer"

type Status struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// IDs are strings for compatibility
type ReportRequest struct {
	Token       string `json:"token" form:"token"`
	Task        string `json:"task" form:"task"`
	UserID      string `json:"user_id" form:"user_id"`
	PipelineID  string `json:"pipeline_id" form:"pipeline_id"`
	ProjectName string `json:"project_name" form:"project_name"`
	Failed      int    `json:"failed,omitempty" form:"failed"`
}

type ReportResponse struct {
	Status
}

type UserScoresRequest struct {
	Login string `json:"login" form:"login"`
}

type UserScoresResponse struct {
	Status

	Scores *scorer.UserScores
}
