//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package telemetry_test

package telemetry

import "context"

type activeTripReader interface {
	CountActive(context.Context) (int64, error)
}
