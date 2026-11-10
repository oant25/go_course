package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/oant25/go_course/internal/http/domainerror"
)

const maxBodyBytes = 1 << 20

var errNotSingleJSONValue = errors.New("request body must contain a single JSON value")

func SingleJSONBody(problems *domainerror.Responder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody || !isJSON(r.Header.Get("Content-Type")) {
				next.ServeHTTP(w, r)
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
			if err != nil {
				problems.WriteInvalidRequest(w, r, err)
				return
			}
			if err := checkSingleJSONValue(body); err != nil {
				problems.WriteInvalidRequest(w, r, err)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
		})
	}
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
