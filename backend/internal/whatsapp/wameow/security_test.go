package wameow

import "testing"

func TestProtocolTablesHaveRLS(t *testing.T) {
	c, ctx := newContainer(t)
	for range 2 {
		if _, err := c.db.Exec(ctx, protectStoreTables); err != nil {
			t.Fatal(err)
		}
	}
	var total, protected int
	if err := c.db.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE c.relrowsecurity) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='r' AND left(c.relname,10)='whatsmeow_'`).Scan(&total, &protected); err != nil {
		t.Fatal(err)
	}
	if total == 0 || protected != total {
		t.Fatalf("protected %d of %d tables", protected, total)
	}
}
