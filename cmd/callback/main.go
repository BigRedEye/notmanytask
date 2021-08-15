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

	/*
		r.Route("/", func(r chi.Router) {
			r.Use(AuthMiddleware(s.authClient))
			r.Handle("/*", http.FileServer(neuteredFileSystem{statikFS}))
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/select", http.StatusFound)
			})
		})
		r.Route("/legion/add", func(r chi.Router) {
			r.Use(AuthMiddleware(s.authClient))
			r.Handle("/*", http.FileServer(neuteredFileSystem{statikFS}))
			r.Get("/", s.submitLegionForm)
			r.Post("/", s.submitLegion)
		})
		r.Route("/cohort/add", func(r chi.Router) {
			r.Use(AuthMiddleware(s.authClient))
			r.Handle("/*", http.FileServer(neuteredFileSystem{statikFS}))
			r.Get("/", s.submitCohortForm)
			r.Post("/", s.submitCohort)
		})
		r.Route("/select", func(r chi.Router) {
			r.Use(AuthMiddleware(s.authClient))
			r.Handle("/*", http.FileServer(neuteredFileSystem{statikFS}))
			r.Get("/", s.selectCohort)
			r.Get("/cohort/{id}", s.confirmCohort)
		})
		r.Route("/directives", func(r chi.Router) {
			r.Use(AuthMiddleware(s.authClient))
			r.Handle("/*", http.FileServer(neuteredFileSystem{statikFS}))
			r.Get("/", s.hostDirectives)
		})
		r.Handle("/auth*", http.FileServer(neuteredFileSystem{statikFS}))
		r.Handle("/static/*", http.FileServer(neuteredFileSystem{statikFS}))
		r.Post("/on_auth", s.onAuth)
	*/

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
