package test_office

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/hngprojects/telex_be/pkg/repository/storage"
	"github.com/hngprojects/telex_be/services/office"
	"github.com/hngprojects/telex_be/utility"
)

// fakeRows answers every query with the same columns and values.
type fakeRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *fakeRows) Columns() []string { return r.columns }

func (r *fakeRows) Close() error { return nil }

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

type fakeStmt struct {
	rows *fakeRows
}

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return -1 }

func (s *fakeStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, io.EOF
}

func (s *fakeStmt) Query([]driver.Value) (driver.Rows, error) {
	return s.rows, nil
}

type fakeConn struct {
	rows *fakeRows
}

func (c *fakeConn) Prepare(string) (driver.Stmt, error) { return &fakeStmt{rows: c.rows}, nil }
func (c *fakeConn) Close() error                        { return nil }
func (c *fakeConn) Begin() (driver.Tx, error)           { return nil, io.EOF }

type fakeDriver struct {
	rows *fakeRows
}

func (d fakeDriver) Open(string) (driver.Conn, error) { return &fakeConn{rows: d.rows}, nil }

// officeTestDB builds a database handle whose queries always answer with the
// given rows, so the office services can be tested without a live Postgres.
func officeTestDB(t *testing.T, columns []string, values [][]driver.Value) *gorm.DB {
	t.Helper()

	name := "office-fake-" + t.Name()
	sql.Register(name, fakeDriver{rows: &fakeRows{columns: columns, values: values}})

	conn, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("failed to open the fake database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:             conn,
		DriverName:       name,
		WithoutReturning: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("failed to build the fake database: %v", err)
	}

	return db
}

func officeService(t *testing.T, db *gorm.DB) *office.Service {
	t.Helper()

	return office.New(&storage.Database{Postgresql: db}, utility.NewLogger())
}

func TestSettingsReadsTheStoredOfficeOptions(t *testing.T) {
	db := officeTestDB(t, []string{"office_settings"}, [][]driver.Value{
		{[]byte(`{"enabled":false,"theme":"dark","capacity":12}`)},
	})

	settings, err := officeService(t, db).Settings("org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.Enabled {
		t.Fatal("the office should be switched off")
	}
	if settings.Theme != "dark" {
		t.Fatalf("theme: got %q, want %q", settings.Theme, "dark")
	}
	if settings.Capacity != 12 {
		t.Fatalf("capacity: got %d, want %d", settings.Capacity, 12)
	}
}

func TestSettingsKeepsTheOfficeOnWhenNothingIsStored(t *testing.T) {
	db := officeTestDB(t, []string{"office_settings"}, [][]driver.Value{{nil}})

	settings, err := officeService(t, db).Settings("org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !settings.Enabled {
		t.Fatal("an organisation that never set the option keeps the office")
	}
}

func TestSettingsKeepsTheOfficeOnWhenTheColumnIsEmpty(t *testing.T) {
	db := officeTestDB(t, []string{"office_settings"}, [][]driver.Value{{[]byte("")}})

	settings, err := officeService(t, db).Settings("org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !settings.Enabled {
		t.Fatal("an organisation that never set the option keeps the office")
	}
}
