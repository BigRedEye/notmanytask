package web

import (
	"fmt"
	"log"

	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func Run(logger *zap.Logger) error {
	config, err := ParseConfig()
	if err != nil {
		return err
	}

	log.Printf("Parsed config: %+v", config)

	db, err := database.OpenDataBase(fmt.Sprintf("postgresql://%s:%s@%s:%d/%s",
		config.Postgres.Username,
		config.Postgres.Password,
		config.Postgres.Host,
		config.Postgres.Port,
		config.Postgres.DataBase,
	))
	if err != nil {
		return errors.Wrap(err, "Failed to open database")
	}

	s, err := newServer(config, logger, db)
	if err != nil {
		return errors.Wrap(err, "Failed to start server")
	}

	return errors.Wrap(s.run(), "Server failed")
}
