package web

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestLoginValidation(t *testing.T) {
	assert.True(t, nameRe.MatchString(`Артём`))
	assert.True(t, !nameRe.MatchString(`Mikail`))
}
