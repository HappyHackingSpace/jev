package jev

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// maxErrorMessage bounds how much of an error body ends up in an error string.
const maxErrorMessage = 1000

// APIError is a non-2xx response from the API.
type APIError struct {
	StatusCode int
	// Message is a readable summary of the error body.
	Message string
	// Body is the raw error body.
	Body []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("typesafe: %d %s: %s", e.StatusCode, http.StatusText(e.StatusCode), e.Message)
}

// Retryable reports whether the same request may succeed later.
func (e *APIError) Retryable() bool {
	return e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

func newAPIError(status int, body []byte) *APIError {
	return &APIError{StatusCode: status, Message: errorMessage(body), Body: body}
}

// errorMessage flattens the two documented error shapes: a string or object
// detail (400), and a list of field errors (422).
func errorMessage(body []byte) string {
	var parsed struct {
		Detail json.RawMessage `json:"detail"`
	}
	msg := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &parsed) == nil && len(parsed.Detail) > 0 {
		msg = detailMessage(parsed.Detail)
	}
	if len(msg) > maxErrorMessage {
		msg = msg[:maxErrorMessage] + "…"
	}
	return msg
}

func detailMessage(detail json.RawMessage) string {
	var text string
	if json.Unmarshal(detail, &text) == nil {
		return text
	}
	var fields []struct {
		Loc []any  `json:"loc"`
		Msg string `json:"msg"`
	}
	if json.Unmarshal(detail, &fields) == nil && len(fields) > 0 {
		parts := make([]string, 0, len(fields))
		for _, f := range fields {
			loc := make([]string, 0, len(f.Loc))
			for _, l := range f.Loc {
				loc = append(loc, fmt.Sprint(l))
			}
			parts = append(parts, strings.Join(loc, ".")+": "+f.Msg)
		}
		return strings.Join(parts, "; ")
	}
	var obj struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(detail, &obj) == nil && obj.Message != "" {
		return obj.Message
	}
	return string(detail)
}
