//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package transaction

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type beginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}
