package api

import "github.com/bigredeye/notmanytask/internal/scorer"

type StandingsRequest struct {
}

type StandingsResponse struct {
	Status

	Standings *scorer.Standings `json:"Standings,omitempty"`
}
