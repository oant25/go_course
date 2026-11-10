package domainerror

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/oant25/go_course/internal/http/requeststate"
)

type Responder struct{}

func NewResponder() *Responder { return &Responder{} }

func (p *Responder) Write(w http.ResponseWriter, r *http.Request, err error) {
	p.write(w, r, err, nil)
}

func (p *Responder) WriteInvalidRequest(w http.ResponseWriter, r *http.Request, _ error) {
	p.Write(w, r, ErrInvalidRequest)
}

func (p *Responder) WritePanic(w http.ResponseWriter, r *http.Request, stack []byte) {
	p.write(w, r, ErrInternal, stack)
}

func (p *Responder) write(w http.ResponseWriter, r *http.Request, err error, stack []byte) {
	status, body := Problem(err)
	instance := r.URL.Path
	body.Instance = &instance
	allow := w.Header().Get("Allow")
	if resetter, ok := w.(interface{ Reset() }); ok {
		resetter.Reset()
	}
	if status == http.StatusMethodNotAllowed && allow != "" {
		w.Header().Set("Allow", allow)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	writeErr := json.NewEncoder(w).Encode(body)
	if writeErr != nil {
		if resetter, ok := w.(interface{ Reset() }); ok {
			resetter.Reset()
			status, body = Problem(ErrInternal)
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(status)
			if fallbackErr := json.NewEncoder(w).Encode(body); fallbackErr != nil {
				writeErr = fmt.Errorf("write fallback problem: %w", fallbackErr)
			}
		}
	}
	if writeErr != nil {
		err = fmt.Errorf("serialize problem: %w", writeErr)
	}
	requeststate.Fail(r.Context(), status, body.Code, err, stack)
}
