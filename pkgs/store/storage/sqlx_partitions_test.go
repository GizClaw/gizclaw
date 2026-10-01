package storage

import (
	"testing"
	"time"
)

func TestDailyPartitionUsesUTCAndNanosecondBounds(t *testing.T) {
	table := SQLTable{dialect: SQLDialectPostgreSQL, kind: "sql", name: "hours", quoted: `"hours"`}
	p := SQLDailyPartitions{Table: table, Column: "hour_unix_nano", Prefix: "hours"}
	at := time.Date(2026, 10, 1, 1, 0, 0, 0, time.FixedZone("SG", 8*3600))
	child, err := p.Partition(at)
	if err != nil {
		t.Fatal(err)
	}
	if child.Name != "hours_p20260930" || child.Upper-child.Lower != int64(24*time.Hour) {
		t.Fatalf("child=%+v", child)
	}
	p.Column = "hour;drop"
	if _, err := p.Partition(at); err == nil {
		t.Fatal("invalid partition column accepted")
	}
	p.Column = "hour"
	p.Table.dialect = SQLDialectSQLite
	if _, err := p.Partition(at); err == nil {
		t.Fatal("SQLite partition accepted")
	}
}
