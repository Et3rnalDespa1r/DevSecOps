package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"ready", nil, http.StatusOK, "{\"status\":\"ok\"}\n"},
		{"database unavailable", errors.New("password=private host=internal"), http.StatusServiceUnavailable, "{\"status\":\"unavailable\"}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			handler := newHandler(func(ctx context.Context) error {
				calls++
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > time.Second {
					t.Error("database check must have a one-second deadline")
				}
				return tt.err
			})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if response.Code != tt.status || response.Body.String() != tt.body {
				t.Errorf("got %d %q, want %d %q", response.Code, response.Body.String(), tt.status, tt.body)
			}
			if calls != 1 {
				t.Errorf("got %d database checks, want 1", calls)
			}
			for header, want := range map[string]string{
				"Content-Type":           "application/json",
				"Cache-Control":          "no-store",
				"X-Content-Type-Options": "nosniff",
			} {
				if got := response.Header().Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}
		})
	}
}

func TestHealthDeadline(t *testing.T) {
	handler := newHandler(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503 for a database timeout", response.Code)
	}
}

func TestUnavailableRoutes(t *testing.T) {
	handler := newHandler(func(context.Context) error {
		t.Fatal("unexpected database access")
		return nil
	})
	for _, path := range []string{"/", "/inventory", "/catalogue", "/loans", "/healthz/"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusNotFound {
				t.Errorf("got %d, want 404", response.Code)
			}
		})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", response.Code)
	}
}

func TestReadConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://demo@127.0.0.1/inventory?sslmode=disable")
	t.Setenv("PORT", "")
	cfg, err := readConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.address != "127.0.0.1:8080" {
		t.Errorf("unexpected default address %q", cfg.address)
	}
	t.Setenv("PORT", "61538")
	cfg, err = readConfig()
	if err != nil || cfg.address != "127.0.0.1:61538" {
		t.Fatalf("custom port: %q, %v", cfg.address, err)
	}
}

func TestInvalidConfig(t *testing.T) {
	tests := []struct {
		name, database, port, want string
	}{
		{"missing database", "", "8080", "DATABASE_URL is required"},
		{"bad database", "postgres://demo:private@invalid:port/db", "8080", "DATABASE_URL is invalid"},
		{"zero port", "postgres://demo@127.0.0.1/inventory", "0", "PORT must be between 1 and 65535"},
		{"negative port", "postgres://demo@127.0.0.1/inventory", "-1", "PORT must be between 1 and 65535"},
		{"large port", "postgres://demo@127.0.0.1/inventory", "65536", "PORT must be between 1 and 65535"},
		{"address as port", "postgres://demo@127.0.0.1/inventory", "0.0.0.0:8080", "PORT must be between 1 and 65535"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tt.database)
			t.Setenv("PORT", tt.port)
			_, err := readConfig()
			if err == nil || err.Error() != tt.want {
				t.Errorf("got %v, want %q", err, tt.want)
			}
			if err != nil && strings.Contains(err.Error(), "private") {
				t.Error("configuration error disclosed credentials")
			}
		})
	}
}
