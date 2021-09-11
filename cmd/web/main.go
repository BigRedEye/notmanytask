package main

import (
	"log"

	_ "github.com/joho/godotenv/autoload"

	"github.com/bigredeye/notmanytask/internal/web"
)

func run() (err error) {
	return web.Run()
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("%+v\n", err)
	}
}
