package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

const maxBodyBytes = 1 << 20

var errNotSingleJSONValue = errors.New("request body must contain a single JSON value")

func singleJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || r.Body == http.NoBody || !isJSON(r.Header.Get("Content-Type")) {
			next.ServeHTTP(w, r)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			writeProblem(w, r, problemInvalidRequest, "can't read request body: "+err.Error())
			return
		}
		if err := checkSingleJSONValue(body); err != nil {
			writeProblem(w, r, problemInvalidRequest, err.Error())
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

func checkSingleJSONValue(data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var v json.RawMessage
	if err := dec.Decode(&v); err != nil {
		return err
	}
	if err := dec.Decode(&v); !errors.Is(err, io.EOF) {
		return errNotSingleJSONValue
	}
	return nil
}

func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}
