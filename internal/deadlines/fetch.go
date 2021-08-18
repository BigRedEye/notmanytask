package deadlines

import (
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
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
