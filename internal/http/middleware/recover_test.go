package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/http/domainerror"
	"github.com/oant25/go_course/internal/http/middleware"
)

func TestRecoverDiscardsPreparedSuccess(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := middleware.Recover(domainerror.NewResponder(), logger, 1024)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/partial")
		w.WriteHeader(201)
		if _, err := w.Write([]byte(`{"partial":`)); err != nil {
			panic(err)
		}
		panic("private-panic-marker")
	}))
	got := httptest.NewRecorder()
	handler.ServeHTTP(got, httptest.NewRequest(http.MethodPost, "/api/v1/trips", nil))
	if got.Code != 500 || got.Header().Get("Location") != "" {
		t.Fatalf("status=%d headers=%v", got.Code, got.Header())
	}
	var body api.Problem
	if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "internal_error" {
		t.Fatalf("problem=%+v", body)
	}
	if strings.Contains(got.Body.String()+logs.String(), "private-panic-marker") {
		t.Fatal("panic value leaked")
	}
	if !strings.Contains(logs.String(), `"stack"`) {
		t.Fatal("panic stack missing")
	}
}
