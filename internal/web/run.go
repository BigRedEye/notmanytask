package web

import (
	"fmt"
	"log"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func Run(logger *zap.Logger) error {
	config, err := config.ParseConfig()
	if err != nil {
		return err
	}

	log.Printf("Parsed config: %+v", config)

	db, err := database.OpenDataBase(fmt.Sprintf("postgresql://%s:%s@%s:%d/%s",
		config.DataBase.User,
		config.DataBase.Pass,
		config.DataBase.Host,
		config.DataBase.Port,
		config.DataBase.Name,
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
