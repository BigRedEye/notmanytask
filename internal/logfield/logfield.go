package lf

import "go.uber.org/zap"

const (
	FieldModule      = "module"
	FieldToken       = "token"
	FieldUserID      = "user_id"
	FieldGitlabID    = "gitlab_id"
	FieldGitlabLogin = "gitlab_login"
	FieldProjectName = "project_name"
	FieldProjectID   = "project_id"
	FieldPipelineID  = "pipeline_id"
)

func Module(module string) zap.Field {
	return zap.String(FieldModule, module)
}

func Token(token string) zap.Field {
	return zap.String(FieldToken, token)
}

func UserID(ID uint) zap.Field {
	return zap.Uint(FieldUserID, ID)
}

func GitlabID(ID int) zap.Field {
	return zap.Int(FieldGitlabID, ID)
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
