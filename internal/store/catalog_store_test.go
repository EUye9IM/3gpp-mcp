package store

import (
	"path/filepath"
	"testing"
)

func openTestStoreDB(t *testing.T) (*DB, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	return db, dbPath
}

func TestCatalogStore_List_Empty(t *testing.T) {
	db, _ := openTestStoreDB(t)
	defer db.Close()

	store := NewCatalogStore(db, "", "")

	specs, err := store.List("", "")
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("empty catalog: want 0, got %d", len(specs))
	}
}

func TestCatalogStore_SyncAndList(t *testing.T) {
	db, _ := openTestStoreDB(t)
	defer db.Close()

	store := NewCatalogStore(db, "", "")

	// Insert test data directly
	tx, err := db.Driver().Begin()
	if err != nil {
		t.Fatal(err)
	}
	testSpecs := []struct {
		id, title, series, wg, version string
	}{
		{"38.331", "NR; Radio Resource Control (RRC); Protocol specification", "38", "R2", "19.3.0"},
		{"23.501", "System architecture for the 5G System (5GS)", "23", "SA", "20.0.0"},
		{"38.304", "NR; UE procedures in Idle mode and RRC Inactive state", "38", "R2", "19.0.0"},
		{"23.502", "Procedures for the 5G System (5GS)", "23", "SA", "20.0.0"},
	}
	for _, sp := range testSpecs {
		if _, err := tx.Exec(
			"INSERT OR REPLACE INTO spec_catalog (id, title, series, wg, version) VALUES (?,?,?,?,?)",
			sp.id, sp.title, sp.series, sp.wg, sp.version,
		); err != nil {
			t.Fatal(err)
		}
	}
	tx.Commit()

	t.Run("list all", func(t *testing.T) {
		specs, err := store.List("", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(specs) != 4 {
			t.Errorf("want 4 specs, got %d", len(specs))
		}
	})

	t.Run("list by series", func(t *testing.T) {
		specs, err := store.List("38", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(specs) != 2 {
			t.Errorf("series 38: want 2, got %d", len(specs))
		}
	})

	t.Run("list by keyword", func(t *testing.T) {
		specs, err := store.List("", "RRC")
		if err != nil {
			t.Fatal(err)
		}
		if len(specs) != 2 {
			t.Errorf("keyword RRC (matches 38.331 and 38.304): want 2, got %d", len(specs))
		}
	})

	t.Run("list by series and keyword", func(t *testing.T) {
		specs, err := store.List("23", "architecture")
		if err != nil {
			t.Fatal(err)
		}
		if len(specs) != 1 {
			t.Errorf("23+architecture: want 1, got %d", len(specs))
		}
	})

	t.Run("get hit", func(t *testing.T) {
		sp, err := store.Get("38.331")
		if err != nil {
			t.Fatal(err)
		}
		if sp == nil {
			t.Fatal("38.331 not found")
		}
		if sp.Title != "NR; Radio Resource Control (RRC); Protocol specification" {
			t.Errorf("wrong title: %s", sp.Title)
		}
	})

	t.Run("get miss", func(t *testing.T) {
		sp, err := store.Get("99.999")
		if err != nil {
			t.Fatal(err)
		}
		if sp != nil {
			t.Error("expected nil for missing spec")
		}
	})
}

func TestParseDynareport(t *testing.T) {
	html := `<html><body>
<table>
  <tr class="even">
    <td>TS</td>
    <td><a href="/DynaReport/38331.htm" target="_blank">38.331</a></td>
    <td>NR; Radio Resource Control (RRC); Protocol specification</td>
    <td>19.3.0</td>
    <td>R2</td>
  </tr>
  <tr class="odd">
    <td>TS</td>
    <td><a href="/DynaReport/23501.htm" target="_blank">23.501</a></td>
    <td>System architecture for the 5G System (5GS)</td>
    <td>20.0.0</td>
    <td>S2</td>
  </tr>
  <tr class="even">
    <td>TR</td>
    <td><a href="/DynaReport/38801.htm" target="_blank">38.801</a></td>
    <td>Study on new radio access technology</td>
    <td>14.0.0</td>
    <td>R2</td>
  </tr>
  <tr class="odd">
    <td>TS</td>
    <td><a href="/DynaReport/38331.htm" target="_blank">38.331</a></td>
    <td>NR; Radio Resource Control (RRC); Protocol specification</td>
    <td>19.3.0</td>
    <td>R2</td>
  </tr>
</table>
</body></html>`

	specs, err := parseDynareport(html)
	if err != nil {
		t.Fatalf("parseDynareport: %v", err)
	}

	if len(specs) != 3 {
		t.Errorf("want 3 specs (deduplicated), got %d", len(specs))
	}

	// Check 38.331
	if specs[0].ID != "38.331" {
		t.Errorf("first spec: want 38.331, got %s", specs[0].ID)
	}
	if specs[0].Series != "38" {
		t.Errorf("series: want 38, got %s", specs[0].Series)
	}
	if specs[0].WG != "R2" {
		t.Errorf("wg: want R2, got %s", specs[0].WG)
	}

	// Check TR
	if specs[2].ID != "38.801" {
		t.Errorf("third spec: want 38.801, got %s", specs[2].ID)
	}
}

func TestParseDynareport_Empty(t *testing.T) {
	_, err := parseDynareport("<html></html>")
	if err == nil {
		t.Fatal("expected error for empty HTML")
	}
}

func TestSync_Integration(t *testing.T) {
	t.Skip("integration test: requires network access")
	db, _ := openTestStoreDB(t)
	defer db.Close()

	store := NewCatalogStore(db, "Mozilla/5.0 (compatible; 3gpp-mcp-test)", "https://www.3gpp.org/dynareport?code=status-report.htm")

	n, err := store.Sync()
	if err != nil {
		t.Fatalf("Sync(): %v", err)
	}
	t.Logf("synced %d specs", n)

	if n < 1000 {
		t.Errorf("expected >1000 specs, got %d", n)
	}
}
