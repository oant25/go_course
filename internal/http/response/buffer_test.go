package response_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/oant25/go_course/internal/http/response"
)

func TestBufferPreservesFirstFinalStatusAndHeaders(t *testing.T) {
	t.Parallel()
	buffer := response.New(4)
	buffer.Header().Add("X-Test", "one")
	buffer.Header().Add("X-Test", "two")
	buffer.WriteHeader(103)
	buffer.WriteHeader(201)
	buffer.WriteHeader(500)
	if n, err := buffer.Write([]byte("body")); n != 4 || err != nil {
		t.Fatalf("write=%d %v", n, err)
	}
	recorder := httptest.NewRecorder()
	if err := buffer.Commit(recorder); err != nil {
		t.Fatal(err)
	}
	buffer.Header()["X-Test"][0] = "changed"
	if recorder.Code != 201 || recorder.Body.String() != "body" || recorder.Header().Values("X-Test")[0] != "one" || len(recorder.Header().Values("X-Test")) != 2 {
		t.Fatalf("commit=%d %v %s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
}

func TestBufferOverflowCannotCommitPartialResponse(t *testing.T) {
	t.Parallel()
	buffer := response.New(4)
	buffer.Header().Set("Location", "/created")
	buffer.WriteHeader(201)
	if _, err := buffer.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if n, err := buffer.Write([]byte("5")); n != 0 || !errors.Is(err, response.ErrTooLarge) {
		t.Fatalf("overflow=%d %v", n, err)
	}
	if _, err := buffer.Write(nil); !errors.Is(err, response.ErrTooLarge) {
		t.Fatalf("overflow not sticky: %v", err)
	}
	recorder := httptest.NewRecorder()
	if err := buffer.Commit(recorder); !errors.Is(err, response.ErrTooLarge) {
		t.Fatalf("commit=%v", err)
	}
	if recorder.Body.Len() != 0 || recorder.Header().Get("Location") != "" {
		t.Fatal("partial response escaped")
	}
	buffer.Reset()
	if buffer.Err() != nil || buffer.Header().Get("Location") != "" {
		t.Fatal("reset retained failure or headers")
	}
	buffer.WriteHeader(500)
	if _, err := buffer.Write([]byte("oops")); err != nil {
		t.Fatal(err)
	}
	if err := buffer.Commit(recorder); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 500 || recorder.Body.String() != "oops" {
		t.Fatalf("reset response=%d %s", recorder.Code, recorder.Body.String())
	}
}

func TestBufferDefaultsToOK(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", "ok"} {
		buffer := response.New(4)
		if body != "" {
			if _, err := buffer.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
		recorder := httptest.NewRecorder()
		if err := buffer.Commit(recorder); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != 200 || recorder.Body.String() != body {
			t.Fatalf("default response=%d %q", recorder.Code, recorder.Body.String())
		}
	}
}
