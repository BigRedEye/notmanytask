package main

import (
	"log"

	zlog "github.com/bigredeye/notmanytask/pkg/log"
	_ "github.com/joho/godotenv/autoload"

	"github.com/bigredeye/notmanytask/internal/web"
)

func run() error {
	logger := zlog.InitDev()
	defer zlog.Sync()

	return web.Run(logger)
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("%+v\n", err)
	}
}
