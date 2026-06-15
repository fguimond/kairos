package client

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is returned when the Kairos API responds with a non-2xx status. The
// Message is taken from the API's {"error": "..."} body when available.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("kairos: %d %s", e.StatusCode, http.StatusText(e.StatusCode))
	}
	return fmt.Sprintf("kairos: %d %s", e.StatusCode, e.Message)
}

// IsNotFound reports whether err is an APIError with a 404 status.
func IsNotFound(err error) bool {
	return hasStatus(err, http.StatusNotFound)
}

// IsUnauthorized reports whether err is an APIError with a 401 status.
func IsUnauthorized(err error) bool {
	return hasStatus(err, http.StatusUnauthorized)
}

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}
