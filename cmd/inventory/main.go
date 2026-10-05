package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type config struct {
	address  string
	database *pgxpool.Config
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func readConfig() (config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}
	database, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return config{}, errors.New("DATABASE_URL is invalid")
	}
	database.MaxConns = 4
	database.ConnConfig.ConnectTimeout = time.Second

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return config{}, errors.New("PORT must be between 1 and 65535")
	}
	return config{address: net.JoinHostPort("127.0.0.1", strconv.Itoa(number)), database: database}, nil
}

func run(ctx context.Context) error {
	cfg, err := readConfig()
	if err != nil {
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg.database)
	if err != nil {
		return errors.New("could not initialize database pool")
	}
	defer pool.Close()

	listener, err := net.Listen("tcp", cfg.address)
	if err != nil {
		return fmt.Errorf("could not listen on %s: %w", cfg.address, err)
	}
	server := &http.Server{
		Handler:           newHandler(pool.Ping),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	log.Printf("listening on http://%s", cfg.address)

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server stopped: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			server.Close()
			return fmt.Errorf("could not finish HTTP requests: %w", err)
		}
		return nil
	}
}

func newHandler(ping func(context.Context) error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		status, body := http.StatusOK, "{\"status\":\"ok\"}\n"
		if err := ping(ctx); err != nil {
			status, body = http.StatusServiceUnavailable, "{\"status\":\"unavailable\"}\n"
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(status)
		io.WriteString(w, body)
	})
	return mux
}
