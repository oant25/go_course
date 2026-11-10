package telemetry

import (
	"context"
	"io"
	"log/slog"
	"slices"

	"go.opentelemetry.io/otel/trace"
)

type logOperation struct {
	group string
	attrs []slog.Attr
}
type correlatedHandler struct {
	base       slog.Handler
	service    string
	operations []logOperation
}

func NewLogger(writer io.Writer, service string, level slog.Level) *slog.Logger {
	return slog.New(&correlatedHandler{base: slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level}), service: service})
}

func (h *correlatedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *correlatedHandler) Handle(ctx context.Context, record slog.Record) error {
	var traceID, spanID string
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		traceID = span.TraceID().String()
		spanID = span.SpanID().String()
	}
	handler := h.base.WithAttrs([]slog.Attr{slog.String("service", h.service), slog.String("trace_id", traceID), slog.String("span_id", spanID)})
	for _, op := range h.operations {
		if op.group != "" {
			handler = handler.WithGroup(op.group)
		} else {
			handler = handler.WithAttrs(op.attrs)
		}
	}
	clean := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		if attr, ok := cleanAttr(attr); ok {
			clean.AddAttrs(attr)
		}
		return true
	})
	return handler.Handle(ctx, clean)
}

func (h *correlatedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copied := *h
	clean := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		if attr, ok := cleanAttr(attr); ok {
			clean = append(clean, attr)
		}
	}
	copied.operations = append(slices.Clone(h.operations), logOperation{attrs: clean})
	return &copied
}

func (h *correlatedHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	copied := *h
	copied.operations = append(slices.Clone(h.operations), logOperation{group: name})
	return &copied
}

func cleanAttr(attr slog.Attr) (slog.Attr, bool) {
	switch attr.Key {
	case "time", "level", "msg", "service", "trace_id", "span_id":
		return slog.Attr{}, false
	}
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() == slog.KindGroup {
		children := make([]slog.Attr, 0, len(attr.Value.Group()))
		for _, child := range attr.Value.Group() {
			if child, ok := cleanAttr(child); ok {
				children = append(children, child)
			}
		}
		attr.Value = slog.GroupValue(children...)
	}
	return attr, true
}
