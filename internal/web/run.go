package web

import (
	"log"

	"github.com/pkg/errors"
	"go.uber.org/zap"
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
}
