package domainerror_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/http/domainerror"
	"github.com/oant25/go_course/internal/http/response"
)

func TestResponderResetsPartialResponse(t *testing.T) {
	t.Parallel()
	responder := domainerror.NewResponder()
	buffer := response.New(1024)
	buffer.Header().Set("Location", "/should-not-survive")
	buffer.WriteHeader(http.StatusCreated)
	if _, err := buffer.Write([]byte(`{"partial":`)); err != nil {
		t.Fatal(err)
	}
	responder.Write(buffer, httptest.NewRequest(http.MethodPost, "/api/v1/trips", nil), errors.New("password=secret-marker"))
	got := httptest.NewRecorder()
	if err := buffer.Commit(got); err != nil {
		t.Fatal(err)
	}
	if got.Code != 500 || got.Header().Get("Location") != "" || got.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("status=%d headers=%v", got.Code, got.Header())
	}
	var body api.Problem
	if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != internalErrorCode || body.Instance == nil || *body.Instance != "/api/v1/trips" {
		t.Fatalf("body=%+v", body)
	}
	if strings.Contains(got.Body.String(), "secret-marker") {
		t.Fatal("secret leaked")
	}
}

func TestResponderPreservesAllow(t *testing.T) {
	t.Parallel()
	responder := domainerror.NewResponder()
	buffer := response.New(1024)
	buffer.Header().Set("Allow", "GET, POST")
	responder.Write(buffer, httptest.NewRequest(http.MethodDelete, "/api/v1/trips", nil), domainerror.ErrMethodNotAllowed)
	got := httptest.NewRecorder()
	if err := buffer.Commit(got); err != nil {
		t.Fatal(err)
	}
	if got.Code != 405 || got.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("status=%d headers=%v", got.Code, got.Header())
	}
}
