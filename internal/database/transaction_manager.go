package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type transactionKey struct{}

type TransactionManager struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func NewTransactionManager(pool *pgxpool.Pool, timeout time.Duration) *TransactionManager {
	return &TransactionManager{pool: pool, timeout: timeout}
}

func (manager *TransactionManager) Do(ctx context.Context, fn func(context.Context) error) (err error) {
	if transactionFromContext(ctx) != nil {
		return fn(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, manager.timeout)
	defer cancel()

	tx, err := manager.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.WithoutCancel(ctx), manager.timeout)
		defer rollbackCancel()
		rollbackErr := tx.Rollback(rollbackCtx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) && err != nil {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	if err = fn(context.WithValue(ctx, transactionKey{}, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func transactionFromContext(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(transactionKey{}).(pgx.Tx)
	return tx
}
