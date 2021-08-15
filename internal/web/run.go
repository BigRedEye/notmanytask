package web

import (
	// "context"
	// "fmt"
	// "io/ioutil"
	"log"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	// "golang.org/x/oauth2"
	// "golang.org/x/oauth2/gitlab"
)

func Run(logger *zap.Logger) error {
	config, err := ParseConfig()
	if err != nil {
		return err
	}

	log.Printf("Parsed config: %+v", config)

	s, err := newServer(config, logger)
	if err != nil {
		return errors.Wrap(err, "Failed to start server")
	}

	return errors.Wrap(s.run(), "Server failed")

	/*
		ctx := context.Background()
		conf := &oauth2.Config{
			ClientID:     config.GitLab.ClientId,
			ClientSecret: config.GitLab.Secret,
			Scopes:       []string{"read_user"},
			Endpoint:     gitlab.Endpoint,
			RedirectURL:  config.Endpoints.Redirect,
		}

		url := conf.AuthCodeURL("state", oauth2.AccessTypeOnline)
		fmt.Printf("Visit the URL for the auth dialog: %v\n", url)

		var code string
		if _, err := fmt.Scan(&code); err != nil {
			logger.Fatal("Failed: ", zap.Error(err))
		}
		tok, err := conf.Exchange(ctx, code)
		if err != nil {
			logger.Fatal("Failed: ", zap.Error(err))
		}

		client := conf.Client(ctx, tok)
		res, err := client.Get("https://gitlab.com/api/v4/user")
		if err != nil {
			logger.Fatal("Failed: ", zap.Error(err))
		}
		buf, err := ioutil.ReadAll(res.Body)
		if err != nil {
			logger.Fatal("Failed: ", zap.Error(err))
		}
		str := string(buf)
		fmt.Println(str)

		return nil
	*/
}
