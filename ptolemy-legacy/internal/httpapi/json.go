package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSONBody(r *http.Request, dst any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(dst); err == nil {
		return nil
	}

	// Some callers embed Windows paths in raw JSON strings without escaping backslashes.
	fixed := strings.ReplaceAll(string(body), "\\", "\\\\")
	return json.NewDecoder(strings.NewReader(fixed)).Decode(dst)
}
