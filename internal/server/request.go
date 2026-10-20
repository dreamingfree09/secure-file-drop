package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// SQL and small JSON requests have short budgets independent of transfer budgets.
func sqlContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 5*time.Second)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	defer func() { _ = http.NewResponseController(w).SetReadDeadline(time.Time{}) }()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		var large *http.MaxBytesError
		status := http.StatusBadRequest
		if errors.As(err, &large) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, "invalid request body", status)
		return false
	}
	// Unmarshal checks the entire bounded body, including trailing data.
	if json.Unmarshal(body, dst) != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}
