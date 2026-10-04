package web

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/scorer"
)

func TestNavigationWhileAdminRepositoryIsBeingCreated(t *testing.T) {
	s := &server{config: &config.Config{}}
	s.config.Server.Admins = []string{"teacher"}
	login := "teacher"
	links := s.makeLinks(&models.User{GitlabUser: models.GitlabUser{GitlabLogin: &login}})
	if links.Admin == "" || links.Repository != "" || links.Submits != "" {
		t.Fatalf("unexpected links while repository is being created: %+v", links)
	}
	tmpl, err := buildHTMLTemplates(templateFuncs)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"home.tmpl", "flag.tmpl", "standings.tmpl"} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			err := tmpl.ExecuteTemplate(&output, name, map[string]interface{}{
				"Links": links, "Config": s.config,
				"Groups": []GroupLink{}, "GroupConfig": &config.GroupConfig{},
				"Standings": &scorer.Standings{Deadlines: &deadlines.Deadlines{}},
			})
			if err != nil {
				t.Fatal(err)
			}
			html := output.String()
			if !strings.Contains(html, `href="/admin/submissions"`) {
				t.Error("admin navigation is hidden while the repository is being created")
			}
			for _, label := range []string{"<h5>My Repo</h5>", "<h5>Submits</h5>"} {
				if strings.Contains(html, label) {
					t.Errorf("unavailable repository link is visible: %s", label)
				}
			}
		})
	}
}
