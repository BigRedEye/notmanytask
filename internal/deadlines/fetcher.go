package deadlines

import (
	"io/ioutil"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	"github.com/bigredeye/notmanytask/internal/config"
)

func fetch(url string) (Deadlines, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to fetch deadlines")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("Failed to fetch deadlines: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to read response")
	}

	deadlines := Deadlines{}
	err = yaml.Unmarshal(body, &deadlines)
	if err != nil {
		return nil, errors.New("Failed to unmarshal deadlines")
	}

	return deadlines, nil
}

type Fetcher struct {
	current atomic.Value

	config   *config.Config
	interval time.Duration
	logger   *zap.Logger

	stop    chan bool
	updater sync.WaitGroup
}

func NewFetcher(config *config.Config, logger *zap.Logger) (*Fetcher, error) {
	fetcher := &Fetcher{
		config: config,
		logger: logger,
		stop:   make(chan bool),
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

func (f *Fetcher) RunUpdater() {
	f.updater.Add(1)
	defer f.updater.Done()

	tick := time.Tick(f.interval)

	for {
		select {
		case <-f.stop:
			return
		case <-tick:
			f.reload()
		}
	}
}

type deadlinesMap = map[string]*Deadlines

func (f *Fetcher) reload() error {
	f.logger.Debug("Fetching deadlines")

	groupDeadlines := make(deadlinesMap)
	for _, group := range f.config.Groups {
		deadlines, err := fetch(group.DeadlinesURL)
		if err != nil {
			f.logger.Error("Failed to reload deadlines", zap.Error(err))
			return errors.Wrap(err, "Failed to reload deadlines")
		}
		groupDeadlines[group.Name] = &deadlines
		f.logger.Debug("Sucessfully fetched deadlines", zap.Int("num_task_groups", len(deadlines)), zap.String("group", group.Name))
	}
	f.logger.Debug("Sucessfully fetched all deadlines")

	f.current.Store(groupDeadlines)
	return nil
}

func (f *Fetcher) StopUpdater() {
	f.stop <- true
	f.updater.Wait()
}

func (f *Fetcher) GroupDeadlines(group string) *Deadlines {
	cur := f.current.Load()
	if cur == nil {
		return nil
	}
	groupDeadlines := cur.(deadlinesMap)
	return groupDeadlines[group]
}
