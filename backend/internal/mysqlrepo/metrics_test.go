package mysqlrepo

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	dto "github.com/prometheus/client_model/go"
)

type metricsTestConnector struct{}
type metricsTestDriver struct{}
type metricsTestConn struct{}
type metricsTestTx struct{}
type metricsTestRows struct {
	query string
	sent  bool
}

func (metricsTestConnector) Connect(context.Context) (driver.Conn, error) {
	return metricsTestConn{}, nil
}
func (metricsTestConnector) Driver() driver.Driver          { return metricsTestDriver{} }
func (metricsTestDriver) Open(string) (driver.Conn, error)  { return metricsTestConn{}, nil }
func (metricsTestConn) Close() error                        { return nil }
func (metricsTestConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unused") }
func (metricsTestConn) Begin() (driver.Tx, error)           { return metricsTestTx{}, nil }
func (metricsTestTx) Commit() error                         { return nil }
func (metricsTestTx) Rollback() error                       { return nil }
func (metricsTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if query == "timeout" {
		return nil, context.DeadlineExceeded
	}
	return &metricsTestRows{query: query}, nil
}
func (metricsTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if query == "timeout" {
		return nil, context.DeadlineExceeded
	}
	return driver.RowsAffected(1), nil
}
func (r *metricsTestRows) Columns() []string { return []string{"value"} }
func (r *metricsTestRows) Close() error      { return nil }
func (r *metricsTestRows) Next(dest []driver.Value) error {
	if r.query == "read failure" {
		return errors.New("unexpected result stream failure")
	}
	if r.sent || r.query == "empty" {
		return io.EOF
	}
	r.sent = true
	if r.query == "invalid type" {
		dest[0] = "not-an-integer"
	} else {
		dest[0] = int64(1)
	}
	return nil
}

func downstreamCount(t *testing.T, operation, outcome string) float64 {
	t.Helper()
	var metric dto.Metric
	if err := downstreamRequests.WithLabelValues("mysql", operation, outcome).Write(&metric); err != nil {
		t.Fatal(err)
	}
	return metric.GetCounter().GetValue()
}

func TestDownstreamQueryLifecycleCountsFailuresAndEmptyResults(t *testing.T) {
	db := sql.OpenDB(metricsTestConnector{})
	t.Cleanup(func() { _ = db.Close() })
	store := NewStore(db, false)
	for _, tt := range []struct{ query, outcome string }{
		{"success", "success"}, {"empty", "success"}, {"timeout", "failure"}, {"invalid type", "failure"}, {"read failure", "failure"},
	} {
		t.Run(tt.query, func(t *testing.T) {
			before := downstreamCount(t, "query", tt.outcome)
			rows, err := store.DB.QueryContext(t.Context(), tt.query)
			if err == nil {
				for rows.Next() {
					var value int
					if err = rows.Scan(&value); err != nil {
						break
					}
				}
				_ = rows.Close()
				_ = rows.Close()
			}
			if delta := downstreamCount(t, "query", tt.outcome) - before; delta != 1 {
				t.Fatalf("operation counted %v times", delta)
			}
		})
	}
	before := downstreamCount(t, "query", "success")
	var value int
	if err := store.DB.QueryRowContext(t.Context(), "empty").Scan(&value); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("empty result = %v", err)
	}
	if delta := downstreamCount(t, "query", "success") - before; delta != 1 {
		t.Fatalf("empty SQL read counted %v times", delta)
	}
}

func TestTransactionDeferredRollbackDoesNotCountAfterCommit(t *testing.T) {
	db := sql.OpenDB(metricsTestConnector{})
	t.Cleanup(func() { _ = db.Close() })
	store := NewStore(db, false)
	before := downstreamCount(t, "rollback", "success")
	tx, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("rollback = %v", err)
	}
	if delta := downstreamCount(t, "rollback", "success") - before; delta != 0 {
		t.Fatalf("no-op rollback counted %v times", delta)
	}
}
