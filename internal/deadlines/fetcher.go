package deadlines

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	"github.com/bigredeye/notmanytask/internal/config"
)

func parseV1(body []byte) (*Deadlines, error) {
	assignments := []TaskGroup{}
	err := yaml.Unmarshal(body, &assignments)
	if err != nil {
		return nil, errors.New("Failed to unmarshal deadlines")
	}

	deadlines := &Deadlines{
		Assignments: dropUnpublished(assignments),
		Scoring: Scoring{
			Policies: []ScoringPolicySpec{{
				Name: "default",
				Kind: "exp",
				Policy: &ExponentialScore{
					Multiplier: time.Hour * 24 * 5,
					Threshold:  0.3,
				}},
			},
			Groups: []ScoringGroup{{
				Name:   "default",
				Weight: 10.0,
				Policy: "default",
			}},
			DefaultGroup: "default",
		},
	}

	err = deadlines.BuildScoringGroups()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to build scoring groups")
	}

	return deadlines, nil
}

func parseV2(body []byte) (*Deadlines, error) {
	deadlines := &Deadlines{}
	err := yaml.Unmarshal(body, &deadlines)
	if err != nil {
		return nil, errors.New("Failed to unmarshal deadlines")
	}
	deadlines.Assignments = dropUnpublished(deadlines.Assignments)

	err = deadlines.BuildScoringGroups()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to build scoring groups")
	}

	return deadlines, nil
}

func fetch(url, format string, headers map[string]string) (*Deadlines, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to build deadlines request")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to fetch deadlines")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to fetch deadlines: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to read response")
	}

	switch format {
	case "v1":
		return parseV1(body)
	case "v2":
		return parseV2(body)
	default:
		return parseV1(body)
	}
}

type Fetcher struct {
	current atomic.Value

	config *config.Config
	logger *zap.Logger
}

func NewFetcher(conf *config.Config, logger *zap.Logger) (*Fetcher, error) {
	fetcher := &Fetcher{
		config: conf,
		logger: logger,
	}

	err := fetcher.reload()
	if err != nil {
		return nil, err
	}

	if fetcher.current.Load() == nil {
		panic("No deadlines found after reload")
	}

	return fetcher, nil
}

func (f *Fetcher) Run(ctx context.Context) {
	tick := time.NewTicker(f.config.PullIntervals.Deadlines)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_ = f.reload()
		}
	}
}

type deadlinesMap = map[string]*Deadlines

func (f *Fetcher) reload() error {
	f.logger.Debug("Start deadlines fetcher iteration")
	defer f.logger.Debug("Finish deadlines fetcher iteration")

	groupDeadlines := make(deadlinesMap)
	for _, group := range f.config.Groups {
		deadlines, err := fetch(group.DeadlinesURL, group.DeadlinesFormat, group.DeadlinesHeaders)
		if err != nil {
			f.logger.Error("Failed to reload deadlines", zap.Error(err))
			return errors.Wrap(err, "Failed to reload deadlines")
		}
		groupDeadlines[group.Name] = deadlines
		f.logger.Debug("Successfully fetched deadlines",
			zap.Int("num_task_groups", len(deadlines.Assignments)),
			zap.String("group", group.Name),
		)
	}
	f.logger.Debug("Successfully fetched all deadlines")

	prev := f.current.Swap(groupDeadlines)
	if !reflect.DeepEqual(prev, groupDeadlines) {
		f.logger.Info("Updated deadlines")
	}

	return nil
}

func (f *Fetcher) GroupDeadlines(group string) *Deadlines {
	cur := f.current.Load()
	if cur == nil {
		return nil
	}
	groupDeadlines := cur.(deadlinesMap)
	return groupDeadlines[group]
}

func (f *Fetcher) AnyGroupHasTask(task string) bool {
	cur := f.current.Load()
	if cur == nil {
		return false
	}
	groupDeadlines := cur.(deadlinesMap)
	for _, deadlines := range groupDeadlines {
		if deadlines.HasTask(task) {
			return true
		}
	}
	return false
}
