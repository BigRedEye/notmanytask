package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/pkg/errors"
	"github.com/rakyll/statik/fs"
	"go.uber.org/zap"

	_ "github.com/bigredeye/notmanytask/pkg/statik"
)

type server struct {
	config *Config
	logger *zap.Logger
}

func newServer(config *Config, logger *zap.Logger) (*server, error) {
	return &server{config, logger}, nil
}

func loggingMiddleware(l *zap.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		handler := func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqid := middleware.GetReqID(r.Context())
			l.Info("Starting request",
				zap.String("proto", r.Proto),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("reqid", reqid),
			)

			writer := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				l.Info("Served",
					zap.String("proto", r.Proto),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Duration("latency", time.Since(start)),
					zap.Int("status", writer.Status()),
					zap.Int("size", writer.BytesWritten()),
					zap.String("reqid", reqid))
			}()

			next.ServeHTTP(writer, r)
		}
		return http.HandlerFunc(handler)
	}
}

func (s *server) run() error {
	r := chi.NewMux()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(loggingMiddleware(s.logger))

	statikFS, err := fs.New()
	if err != nil {
		return errors.Wrap(err, "Failed to open statik fs")
	}

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Hello, world!")
	})
	r.Handle("/kek*", http.FileServer(statikFS))
	s.logger.Info("Starting server", zap.String("bind_address", s.config.Server.ListenAddress))
	return http.ListenAndServe(s.config.Server.ListenAddress, r)
}
