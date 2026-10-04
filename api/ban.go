package api

// BanRequest excludes a pipeline from scoring (or restores it). It is
// protected by the same token as OverrideRequest.
type BanRequest struct {
	PipelineID int    `json:"pipeline_id" form:"pipeline_id"`
	Reason     string `json:"reason,omitempty" form:"reason"`
}

type BanResponse struct {
	Status
}
