package main

import (
	"log"

	zlog "github.com/bigredeye/notmanytask/pkg/log"
	_ "github.com/joho/godotenv/autoload"

	"github.com/bigredeye/notmanytask/internal/web"
)

func run() (err error) {
	logger := zlog.InitDev()
	defer func() {
		err = zlog.Sync()
	}()

	err = web.Run(logger)
	return
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("%+v\n", err)
	}
}
