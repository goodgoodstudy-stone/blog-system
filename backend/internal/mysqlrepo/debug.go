package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"reflect"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"
)

// DB keeps raw SQL diagnostics local to the API process. Queries remain
// parameterized; sql and args are logged separately because database/sql does
// not expose an interpolated statement.
type DB struct {
	*sql.DB
	raw bool
}

func NewStore(db *sql.DB, raw bool) Store {
	return Store{DB: &DB{DB: db, raw: raw}}
}

var querySequence atomic.Uint64

func debugSQL(ctx context.Context, msg string, attrs ...any) {
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		attrs = append(attrs, "traceId", span.TraceID().String(), "spanId", span.SpanID().String())
	}
	slog.InfoContext(ctx, msg, attrs...)
}

func scannedValues(dest []any) []any {
	values := make([]any, len(dest))
	for i, value := range dest {
		ref := reflect.ValueOf(value)
		if ref.IsValid() && ref.Kind() == reflect.Pointer && !ref.IsNil() {
			values[i] = ref.Elem().Interface()
		} else {
			values[i] = value
		}
	}
	return values
}

type Row struct {
	*sql.Row
	ctx     context.Context
	queryID uint64
	call    *downstreamCall
	raw     bool
}

func (r *Row) Scan(dest ...any) error {
	err := r.Row.Scan(dest...)
	var attrs []any
	if err == nil {
		r.call.rows = 1
		if r.raw {
			attrs = []any{"values", scannedValues(dest)}
		}
	}
	r.call.finish(err, attrs...)
	if r.raw {
		if errors.Is(err, sql.ErrNoRows) {
			debugSQL(r.ctx, "mysql result", "queryId", r.queryID, "rows", 0, "empty", true)
		} else if err != nil {
			debugSQL(r.ctx, "mysql result", "queryId", r.queryID, "error", err)
		} else {
			debugSQL(r.ctx, "mysql result", "queryId", r.queryID, "values", scannedValues(dest))
		}
	}
	return err
}

type Rows struct {
	*sql.Rows
	ctx     context.Context
	queryID uint64
	columns []string
	call    *downstreamCall
	count   int
	raw     bool
}

func (r *Rows) Scan(dest ...any) error {
	err := r.Rows.Scan(dest...)
	if err != nil {
		r.call.err = err
	} else {
		r.call.rows++
	}
	if r.raw {
		if err != nil {
			debugSQL(r.ctx, "mysql row", "queryId", r.queryID, "error", err)
		} else {
			r.count++
			debugSQL(r.ctx, "mysql row", "queryId", r.queryID, "row", r.count, "columns", r.columns, "values", scannedValues(dest))
		}
	}
	return err
}

func (r *Rows) Next() bool {
	next := r.Rows.Next()
	if !next {
		r.call.finish(errors.Join(r.call.err, r.Rows.Err()))
	}
	return next
}

func (r *Rows) Err() error {
	err := r.Rows.Err()
	if err != nil {
		r.call.err = err
	}
	return err
}

func (r *Rows) Close() error {
	err := r.Rows.Close()
	r.call.finish(errors.Join(r.call.err, r.Rows.Err(), err))
	if r.raw {
		debugSQL(r.ctx, "mysql result", "queryId", r.queryID, "rows", r.count, "error", err)
	}
	return err
}

func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *Row {
	call := newDownstreamCall(ctx, "query", query, db.raw, args)
	id := call.id
	if db.raw {
		debugSQL(ctx, "mysql query", "queryId", id, "sql", query, "args", args)
	}
	return &Row{Row: db.DB.QueryRowContext(ctx, query, args...), ctx: ctx, queryID: id, raw: db.raw, call: call}
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*Rows, error) {
	call := newDownstreamCall(ctx, "query", query, db.raw, args)
	id := call.id
	if db.raw {
		debugSQL(ctx, "mysql query", "queryId", id, "sql", query, "args", args)
	}
	rows, err := db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		call.finish(err)
		if db.raw {
			debugSQL(ctx, "mysql result", "queryId", id, "error", err)
		}
		return nil, err
	}
	var columns []string
	if db.raw {
		columns, _ = rows.Columns()
	}
	return &Rows{Rows: rows, ctx: ctx, queryID: id, columns: columns, raw: db.raw, call: call}, nil
}

func logExecResult(ctx context.Context, id uint64, result sql.Result, err error) {
	if err != nil {
		debugSQL(ctx, "mysql result", "queryId", id, "error", err)
		return
	}
	affected, affectedErr := result.RowsAffected()
	insertID, insertErr := result.LastInsertId()
	attrs := []any{"queryId", id}
	if affectedErr == nil {
		attrs = append(attrs, "rowsAffected", affected)
	}
	if insertErr == nil {
		attrs = append(attrs, "lastInsertId", insertID)
	}
	debugSQL(ctx, "mysql result", attrs...)
}

func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	call := newDownstreamCall(ctx, "exec", query, db.raw, args)
	id := call.id
	if db.raw {
		debugSQL(ctx, "mysql query", "queryId", id, "sql", query, "args", args)
	}
	result, err := db.DB.ExecContext(ctx, query, args...)
	finishExec(call, result, err)
	if db.raw {
		logExecResult(ctx, id, result, err)
	}
	return result, err
}

type Tx struct {
	*sql.Tx
	raw bool
	ctx context.Context
}

func (db *DB) BeginTx(ctx context.Context, options *sql.TxOptions) (*Tx, error) {
	call := newDownstreamCall(ctx, "begin", "", db.raw, nil)
	tx, err := db.DB.BeginTx(ctx, options)
	call.finish(err)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, raw: db.raw, ctx: ctx}, nil
}

func (tx *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *Row {
	call := newDownstreamCall(ctx, "query", query, tx.raw, args)
	id := call.id
	if tx.raw {
		debugSQL(ctx, "mysql query", "queryId", id, "sql", query, "args", args)
	}
	return &Row{Row: tx.Tx.QueryRowContext(ctx, query, args...), ctx: ctx, queryID: id, raw: tx.raw, call: call}
}

func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	call := newDownstreamCall(ctx, "exec", query, tx.raw, args)
	id := call.id
	if tx.raw {
		debugSQL(ctx, "mysql query", "queryId", id, "sql", query, "args", args)
	}
	result, err := tx.Tx.ExecContext(ctx, query, args...)
	finishExec(call, result, err)
	if tx.raw {
		logExecResult(ctx, id, result, err)
	}
	return result, err
}

func finishExec(call *downstreamCall, result sql.Result, err error) {
	if err != nil {
		call.finish(err)
		return
	}
	affected, affectedErr := result.RowsAffected()
	id, idErr := result.LastInsertId()
	call.finish(errors.Join(affectedErr, idErr), "rowsAffected", affected, "lastInsertId", id)
}

func (tx *Tx) Commit() error {
	call := newDownstreamCall(tx.ctx, "commit", "", tx.raw, nil)
	err := tx.Tx.Commit()
	call.finish(err)
	return err
}

func (tx *Tx) Rollback() error {
	call := newDownstreamCall(tx.ctx, "rollback", "", tx.raw, nil)
	err := tx.Tx.Rollback()
	// Deferred rollback after commit is a no-op, not another DB operation.
	if !errors.Is(err, sql.ErrTxDone) {
		call.finish(err)
	}
	return err
}
