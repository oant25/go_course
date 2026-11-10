package telemetry_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/oant25/go_course/internal/config"
	"github.com/oant25/go_course/internal/infra/telemetry"
)

func TestResourceHasRequiredIdentity(t *testing.T) {
	t.Parallel()
	res := telemetry.Resource("trip-service", map[string]string{"service.namespace": "tripgo", "deployment.environment": "local", "service.version": testIdentity, "service.name": "spoofed"})
	for key, want := range map[string]string{"service.name": "trip-service", "service.namespace": "tripgo", "deployment.environment": "local", "service.version": testIdentity} {
		got, ok := res.Set().Value(attribute.Key(key))
		if !ok || got.AsString() != want {
			t.Fatalf("resource %s=%v", key, got)
		}
	}
}

func TestSamplerHonorsParentAndRatio(t *testing.T) {
	t.Parallel()
	id, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	parentID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	for _, tc := range []struct {
		name  string
		ratio float64
		want  sdktrace.SamplingDecision
	}{
		{"always_on", 0, sdktrace.RecordAndSample},
		{"always_off", 0, sdktrace.Drop},
		{"traceidratio", 0, sdktrace.Drop},
		{"traceidratio", 1, sdktrace.RecordAndSample},
		{"parentbased_always_on", 0, sdktrace.RecordAndSample},
		{"parentbased_always_off", 0, sdktrace.Drop},
		{"parentbased_traceidratio", 0, sdktrace.Drop},
	} {
		sampler := telemetry.Sampler(config.Telemetry{Sampler: tc.name, SamplerRatio: tc.ratio})
		got := sampler.ShouldSample(sdktrace.SamplingParameters{ParentContext: context.Background(), TraceID: id, Name: testIdentity})
		if got.Decision != tc.want {
			t.Fatalf("%s ratio=%v decision=%v", tc.name, tc.ratio, got.Decision)
		}
	}
	sampler := telemetry.Sampler(config.Telemetry{Sampler: "parentbased_always_off"})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: id, SpanID: parentID, TraceFlags: trace.FlagsSampled, Remote: true}))
	if got := sampler.ShouldSample(sdktrace.SamplingParameters{ParentContext: ctx, TraceID: id, Name: testIdentity}); got.Decision != sdktrace.RecordAndSample {
		t.Fatal("sampled remote parent ignored")
	}
}

const (
	testIdentity = "test"
)
