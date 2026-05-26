package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
)

type envelope struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data"`
	Error     *string     `json:"error"`
	ErrorCode string      `json:"error_code,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Success: true, Data: normalizeResponseData(data), Error: nil})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeErrorWithCode(w, status, message, "")
}

func writeErrorWithCode(w http.ResponseWriter, status int, message string, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Success: false, Data: nil, Error: &message, ErrorCode: code})
}

func writeErrorWithData(w http.ResponseWriter, status int, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Success: false, Data: data, Error: &message})
}

func normalizeResponseData(data interface{}) interface{} {
	value := reflect.ValueOf(data)
	if !value.IsValid() || value.Kind() != reflect.Slice || !value.IsNil() {
		return data
	}
	return reflect.MakeSlice(value.Type(), 0, 0).Interface()
}
