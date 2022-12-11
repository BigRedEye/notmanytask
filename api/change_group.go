package api

type ChangeGroupRequest struct {
	Token     string `json:"token" form:"token"`
	Login     string `json:"login" form:"login"`
	GroupName string `json:"group_name" form:"group_name"`
}

type ChangeGroupResponse struct {
	Status
}
