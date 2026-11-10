package config_test

import (
	"strings"
	"testing"

	"github.com/oant25/go_course/internal/config"
)

func TestTelemetryConfigurationValidation(t *testing.T) {
	validEnvironment(t)
	for _, tc := range []struct{ key, value string }{
		{exportEndpointEnv, "collector.invalid:4318"},
		{exportEndpointEnv, "http://user:secret_marker@collector.invalid:4318"},
		{"OTEL_EXPORTER_OTLP_PROTOCOL", "json"},
		{"OTEL_METRIC_EXPORT_TIMEOUT", "10001"},
		{exportTimeoutEnv, "1s"},
		{exportTimeoutEnv, "9223372036854775807"},
		{"OTEL_BSP_MAX_EXPORT_BATCH_SIZE", "2049"},
		{"OTEL_TRACES_SAMPLER", "invalid"},
		{resourceAttributesEnv, "service.namespace=tripgo"},
		{resourceAttributesEnv, "service.namespace=tripgo,deployment.environment=local,service.version=dev,service.version=other"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("validation=%v", err)
			}
			if strings.Contains(err.Error(), "secret_marker") {
				t.Fatal("endpoint credentials leaked")
			}
		})
	}
}

func TestRatioSamplerConfiguration(t *testing.T) {
	validEnvironment(t)
	t.Setenv("OTEL_TRACES_SAMPLER", "parentbased_traceidratio")
	for _, ratio := range []string{"", "NaN", "Inf", "-0.1", "1.1"} {
		t.Run("invalid "+ratio, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", ratio)
			if _, err := config.Load(); err == nil {
				t.Fatal("accepted invalid ratio")
			}
		})
	}
	for _, ratio := range []string{"0", "0.5", "1"} {
		t.Run("valid "+ratio, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", ratio)
			if _, err := config.Load(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
