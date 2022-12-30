package notmanytask

import (
	"fmt"
	"time"

	"github.com/bigredeye/notmanytask/api"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/scorer"
	"github.com/go-resty/resty/v2"
)

type Client struct {
	client *resty.Client
	token  string
}

func NewClient(endpoint, token string) (*Client, error) {
	client := resty.New().
		SetBaseURL(endpoint).
		SetTimeout(time.Second * 10).
		SetRetryCount(3)

	client.Header.Add("Token", token)

	return &Client{client, token}, nil
}

func (c *Client) LoadStandings(group string) (*scorer.Standings, error) {
	res := &api.StandingsResponse{}
	_, err := c.client.R().
		SetResult(res).
		SetQueryParam("group", group).
		Get("/api/standings")
	if err != nil {
		return nil, err
	}

	if !res.Ok {
		return nil, fmt.Errorf("failed to fetch standings: %s", res.Error)
	}

	return res.Standings, nil
}

func (c *Client) LoadUsers(group string) ([]*models.User, error) {
	res := &api.GroupMembers{}
	_, err := c.client.R().
		SetResult(res).
		SetPathParam("group", group).
		Get("/api/group/{group}/members")
	if err != nil {
		return nil, err
	}

	if !res.Ok {
		return nil, fmt.Errorf("failed to fetch group members: %s", res.Error)
	}

	return res.Users, nil
}

func (c *Client) OverrideScore(user, task, status string, score int) error {
	res := &api.GroupMembers{}
	_, err := c.client.R().
		SetResult(res).
		SetBody(api.OverrideRequest{
			Token:  c.token,
			Task:   task,
			Login:  user,
			Score:  score,
			Status: status,
		}).
		Post("/api/override")
	if err != nil {
		return err
	}

	if !res.Ok {
		return fmt.Errorf("failed to override score: %s", res.Error)
	}

	return nil
}

func (c *Client) LoadSuccessfulSubmits(group, taskname string) ([]*scorer.User, error) {
	standings, err := c.LoadStandings(group)
	if err != nil {
		return nil, err
	}

	res := make([]*scorer.User, 0)
	for _, user := range standings.Users {
		for _, grp := range user.Groups {
			for _, task := range grp.Tasks {
				if task.Task != taskname {
					continue
				}

				if task.Status != scorer.TaskStatusSuccess || task.Score <= 0 {
					continue
				}

				res = append(res, &user.User)
			}
		}
	}

	return res, nil
}
