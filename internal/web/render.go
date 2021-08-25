package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *server) RenderSignupPage(c *gin.Context, err string) {
	c.HTML(http.StatusOK, "/signup.tmpl", gin.H{
		"CourseName":   "HSE Advanced C++",
		"Config":       s.config,
		"ErrorMessage": err,
	})
}

func (s *server) RenderHomePage(c *gin.Context) {
	c.HTML(http.StatusOK, "/home.tmpl", gin.H{
		"CourseName":   "HSE Advanced C++",
		"Config":       s.config,
		"ErrorMessage": err,
	})

	c.HTML(http.StatusOK, "/home.tmpl", gin.H{
		"CourseName": "HSE Advanced C++",
	})
}
