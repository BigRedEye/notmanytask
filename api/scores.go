package api

import "github.com/bigredeye/notmanytask/internal/scorer"

type UserScoresRequest struct {
	Login string `json:"login" form:"login"`
}

type UserScoresResponse struct {
	Status

	Scores *scorer.UserScores `json:"scores,omitempty"`
}
