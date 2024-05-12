package base

import (
	"fmt"
	"strings"

	"github.com/alexsergivan/transliterator"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/models"
)

func Main() {
	fmt.Println("vim-go")
}

type ClientInterface interface {
	InitializeProject(user *models.User) error
	CleanupName(name string) string
	CleanupLogin(login string) string
	MakeProjectName(user *models.User) string
	MakeProjectURL(user *models.User) string
	MakeProjectSubmitsURL(user *models.User) string
	MakeProjectWithNamespace(project string) string
	MakePipelineURL(user *models.User, pipeline *models.Pipeline) string
	MakeBranchURL(user *models.User, pipeline *models.Pipeline) string
	MakeTaskURL(task string) string
}

type ClientBase struct {
	Config   *config.Config
	Logger   *zap.Logger
	Translit *transliterator.Transliterator
}

const (
	Master = "master"
)

func (c *ClientBase) CleanupName(name string) string {
	transliteratedName := c.Translit.Transliterate(name, "en")
	return strings.Map(func(ch rune) rune {
		switch ch {
		case '-':
			return -1
		case '\'':
			return -1
		}
		return ch
	}, transliteratedName)
}

func (c *ClientBase) CleanupLogin(login string) string {
	return strings.ReplaceAll(login, "__", "")
}
