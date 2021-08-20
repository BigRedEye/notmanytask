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
)

func Fetch(url string) (Deadlines, error) {
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

	url      string
	interval time.Duration
	logger   *zap.Logger

	stop    chan bool
	updater sync.WaitGroup
}

func NewFetcher(url string, interval time.Duration, logger *zap.Logger) *Fetcher {
	return &Fetcher{
		url:    url,
		stop:   make(chan bool),
		logger: logger,
	}
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

func (f *Fetcher) reload() {
	f.logger.Debug("Fetching deadlines")

	deadlines, err := Fetch(f.url)
	if err != nil {
		f.logger.Error("Failed to reload deadlines", zap.Error(err))
		return
	}

	f.logger.Debug("Sucessfully fetched deadlines", zap.Int("num_groups", len(deadlines)))
	f.current.Store(deadlines)
}

func (f *Fetcher) StopUpdater() {
	f.stop <- true
	f.updater.Wait()
}
