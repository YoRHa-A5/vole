package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// handler wraps the extractClient and provides HTTP endpoints.
type handler struct {
	client *extractClient
}

// newHandler creates a new handler with the given extract client.
func newHandler(client *extractClient) *handler {
	return &handler{client: client}
}

// extractRequest is the JSON body for POST /extract.
type extractRequest struct {
	URL    string `json:"url"`
	Format string `json:"format"`
}

// extractResponse is the JSON response for /extract.
type extractResponse struct {
	ExtractResult
}

// extractErrorResponse is the JSON error response.
type extractErrorResponse struct {
	ExtractError
}

// ServeHTTP routes requests to the appropriate handler.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/extract":
		h.handleExtract(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleExtract processes extraction requests from both POST and GET.
func (h *handler) handleExtract(w http.ResponseWriter, r *http.Request) {
	var req *extractRequest
	var err error

	switch r.Method {
	case http.MethodPost:
		req, err = h.parsePostRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "parse_error", err.Error())
			return
		}
	case http.MethodGet:
		req, err = h.parseGetRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "parse_error", err.Error())
			return
		}
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST and GET are allowed")
		return
	}

	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "missing_url", "url is required")
		return
	}

	result, err := h.client.extract(req.URL)
	if err != nil {
		// Internal errors (fetch failures, parse errors)
		status := http.StatusInternalServerError
		detail := err.Error()

		// Check for fetch failures
		if strings.Contains(detail, "fetch failed") {
			status = http.StatusBadGateway
			detail = fmt.Sprintf("fetch_failed: %s", detail)
		}

		writeError(w, status, "parse_error", detail)
		return
	}

	switch r := result.(type) {
	case *ExtractResult:
		writeJSON(w, http.StatusOK, extractResponse{*r})
	case *ExtractError:
		status := http.StatusInternalServerError
		switch r.Error {
		case "unsupported_content_type":
			status = http.StatusUnsupportedMediaType
		case "upstream_error":
			if r.StatusCode != nil {
				status = *r.StatusCode
			} else {
				status = http.StatusBadGateway
			}
		}
		writeError(w, status, r.Error, r.Detail)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "unknown response type")
	}
}

// parsePostRequest parses a POST request body.
func (h *handler) parsePostRequest(r *http.Request) (*extractRequest, error) {
	var req extractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	return &req, nil
}

// parseGetRequest parses a GET request with query parameters.
func (h *handler) parseGetRequest(r *http.Request) (*extractRequest, error) {
	url := r.URL.Query().Get("url")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "markdown"
	}
	return &extractRequest{
		URL:    url,
		Format: format,
	}, nil
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, errorType, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(extractErrorResponse{
		ExtractError: ExtractError{
			Error:  errorType,
			Detail: detail,
		},
	})
}
