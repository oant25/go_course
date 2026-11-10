package ready_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	api "github.com/oant25/go_course/internal/generated"
	"github.com/oant25/go_course/internal/http/ready"
)

func TestReadiness(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		failure error
		status  int
		body    string
	}{
		{"ready", nil, 200, `{"status":"ok"}`},
		{"unavailable", errors.New("secret_marker"), 503, `{"status":"unavailable"}`},
		{"cancelled", context.Canceled, 503, `{"status":"unavailable"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pinger := NewMockpinger(gomock.NewController(t))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if errors.Is(tc.failure, context.Canceled) {
				cancel()
			}
			var pingCtx context.Context
			pinger.EXPECT().Ping(gomock.Any()).DoAndReturn(func(got context.Context) error {
				pingCtx = got
				deadline, ok := got.Deadline()
				if !ok || time.Until(deadline) > time.Second {
					t.Fatal("missing bounded ping context")
				}
				if errors.Is(tc.failure, context.Canceled) && !errors.Is(got.Err(), context.Canceled) {
					t.Fatal("cancellation not propagated")
				}
				return tc.failure
			})
			result, err := ready.New(pinger, time.Second).Ready(ctx, api.ReadyRequestObject{})
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(pingCtx.Err(), context.Canceled) {
				t.Fatal("ping context not released")
			}
			response := httptest.NewRecorder()
			if err := result.VisitReadyResponse(response); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || strings.TrimSpace(response.Body.String()) != tc.body {
				t.Fatalf("%d %s", response.Code, response.Body.String())
			}
		})
	}
}
