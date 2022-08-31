package lf

import "go.uber.org/zap"

const (
	FieldToken          = "token"
	FieldUserID         = "user_id"
	FieldGitlabID       = "gitlab_id"
	FieldGitlabLogin    = "gitlab_login"
	FieldProjectName    = "project_name"
	FieldProjectID      = "project_id"
	FieldPipelineID     = "pipeline_id"
	FieldPipelineStatus = "pipeline_status"
)

func Token(token string) zap.Field {
	return zap.String(FieldToken, token)
}

func UserID(id uint) zap.Field {
	return zap.Uint(FieldUserID, id)
}

func GitlabID(id int) zap.Field {
	return zap.Int(FieldGitlabID, id)
}

func GitlabLogin(login string) zap.Field {
	return zap.String(FieldGitlabLogin, login)
}

func ProjectName(name string) zap.Field {
	return zap.String(FieldProjectName, name)
}

func ProjectID(ID int) zap.Field {
	return zap.Int(FieldProjectID, ID)
}

func PipelineID(ID int) zap.Field {
	return zap.Int(FieldPipelineID, ID)
}

func PipelineStatus(status string) zap.Field {
	return zap.String(FieldPipelineStatus, status)
}
