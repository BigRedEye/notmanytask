package api

import (
	"github.com/bigredeye/notmanytask/internal/models"
)

type GroupMembers struct {
	Status

	Users []*models.User
}
