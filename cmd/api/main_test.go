package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"spliteasy/internal/handler"

	"github.com/go-chi/chi/v5"
)

func TestPingRoute(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"message": "pong", "status": "ok"}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	})

	req, _ := http.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSwaggerRoute(t *testing.T) {
	r := chi.NewRouter()
	r.Handle("/swagger/*", handler.SwaggerHandler())

	// Test requesting the swagger specification JSON file. httpSwagger reads
	// RequestURI to resolve the resource, which http.NewRequest leaves empty.
	req, _ := http.NewRequest("GET", "/swagger/doc.json", nil)
	req.RequestURI = "/swagger/doc.json"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status code %d when fetching doc.json, got %d", http.StatusOK, w.Code)
	}

	// Verify it contains swagger JSON structure
	var doc map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&doc); err != nil {
		t.Fatalf("Failed to decode swagger JSON: %v", err)
	}

	if doc["swagger"] != "2.0" {
		t.Errorf("Expected swagger version '2.0', got '%v'", doc["swagger"])
	}

	info, ok := doc["info"].(map[string]interface{})
	if !ok || info["title"] != "SplitEasy API" {
		t.Errorf("Expected API title 'SplitEasy API', got '%v'", info["title"])
	}
}
