package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (q *Queries) ExecTx(ctx context.Context, fn func(q *Queries) error) error {
	if _, ok := q.db.(pgx.Tx); ok {
		return fn(q)
	}

	// ожидаем что q.db - *pgxpool.Pool
	pool, ok := q.db.(*pgxpool.Pool)
	if !ok {
		return errors.New("ExecTx: unsupported db type")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	txq := q.WithTx(tx)
	if err := fn(txq); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			// возвращаем оригинальную ошибку, дополняя ошибкой отката
			return fmt.Errorf("%w; rollback error: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
