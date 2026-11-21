// Package apierror writes the one error envelope the HTTP API uses for every
// error response.
package apierror

import (
	"encoding/json"
	"net/http"
)

const (
	CodeUnauthenticated = "unauthenticated"
	CodeForbidden       = "forbidden"
)

type envelope struct {
	Error body `json:"error"`
}

// The error is an object rather than a string so that fields can be added
// later without changing the shape clients already parse.
type body struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Write sends status with the envelope. code is a stable identifier clients
// can branch on; message is for people and may change.
func Write(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Error: body{Code: code, Message: message}})
}
