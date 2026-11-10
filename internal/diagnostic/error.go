// Package diagnostic содержит реализацию пакета diagnostic.
package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

func Kind(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if SQLState(err) != "" {
		return "database_error"
	}
	return "internal_error"
}

func SQLState(err error) string {
	var state interface{ SQLState() string }
	if errors.As(err, &state) {
		value := state.SQLState()
		if len(value) == 5 && strings.IndexFunc(value, func(r rune) bool { return (r < '0' || r > '9') && (r < 'A' || r > 'Z') }) < 0 {
			return value
		}
	}
	return ""
}

func Safe(err error) error {
	if err == nil {
		return nil
	}
	message := Kind(err)
	if state := SQLState(err); state != "" {
		message += " SQLSTATE=" + state
	}
	return errors.New(message)
}

func Attributes(err error) []slog.Attr {
	if err == nil {
		return nil
	}
	chain := make([]string, 0, 8)
	var visit func(error, int)
	visit = func(e error, depth int) {
		if e == nil || depth >= 8 || len(chain) >= 8 {
			return
		}
		chain = append(chain, fmt.Sprintf("%T", e))
		if joined, ok := e.(interface{ Unwrap() []error }); ok {
			for _, child := range joined.Unwrap() {
				visit(child, depth+1)
			}
		} else {
			visit(errors.Unwrap(e), depth+1)
		}
	}
	visit(err, 0)
	attrs := []slog.Attr{slog.String("error_type", Kind(err)), slog.Any("error_chain", chain)}
	if state := SQLState(err); state != "" {
		attrs = append(attrs, slog.String("sqlstate", state))
	}
	return attrs
}
