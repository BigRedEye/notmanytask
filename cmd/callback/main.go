package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	lg "github.com/pressly/lg"
	log "github.com/sirupsen/logrus"

	_ "github.com/bigredeye/notmanytask/pkg/statik"
	"github.com/rakyll/statik/fs"
)

func run() error {
	lg.RedirectStdlogOutput(log.StandardLogger())
	lg.DefaultLogger = log.StandardLogger()

	r := chi.NewMux()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(lg.RequestLogger(log.StandardLogger()))

	statikFS, err := fs.New()
	if err != nil {
		log.WithError(err).Fatal("Failed to open statik fs")
	}

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		log.Infof("GET %s", r.RequestURI)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Hello, world!")
	})
	r.Handle("/kek*", http.FileServer(statikFS))
	log.Infof("Starting server at %s", ":18080")
	return http.ListenAndServe(":18080", r)
}

func main() {
	err := run()
	if err != nil {
		log.Fatalf("Server failed: %v\n", err)
	}
}
