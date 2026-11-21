package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type ctxKey int

const (
	ctxUserID ctxKey = iota
	ctxSessionID
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

// decodeJSON reads a single JSON object of at most 1 MiB, rejecting unknown
// fields and trailing data.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after JSON body")
	}
	return nil
}

// userID returns the authenticated user. It is only set by the authenticate
// middleware, so identifiers sent by the client can never impersonate a user.
func userID(r *http.Request) int64 {
	id, _ := r.Context().Value(ctxUserID).(int64)
	return id
}

func sessionID(r *http.Request) int64 {
	id, _ := r.Context().Value(ctxSessionID).(int64)
	return id
}
