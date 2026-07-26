package store

import (
	"errors"
	"testing"

	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
)

func TestSpecStore_InsertAndGet(t *testing.T) {
	db, _ := openTestStoreDB(t)
	defer db.Close()

	store := NewSpecStore(db)

	sections := []model.Section{
		{SectionNumber: "5", ParentNumber: "", Title: "Top Level", Content: "top content"},
		{SectionNumber: "5.3", ParentNumber: "5", Title: "Subsection", Content: "subsection content"},
		{SectionNumber: "5.3.7", ParentNumber: "5.3", Title: "Leaf", Content: "leaf content"},
	}

	if err := store.InsertSections("38.331", "Rel-18", sections); err != nil {
		t.Fatalf("InsertSections: %v", err)
	}

	t.Run("has content", func(t *testing.T) {
		has, err := store.HasContent("38.331", "Rel-18")
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Error("expected HasContent=true")
		}
	})

	t.Run("has content miss", func(t *testing.T) {
		has, err := store.HasContent("99.999", "Rel-18")
		if err != nil {
			t.Fatal(err)
		}
		if has {
			t.Error("expected HasContent=false")
		}
	})

	t.Run("get section", func(t *testing.T) {
		sec, err := store.GetSection("38.331", "Rel-18", "5.3.7")
		if err != nil {
			t.Fatal(err)
		}
		if sec.Title != "Leaf" {
			t.Errorf("title: want Leaf, got %q", sec.Title)
		}
		if sec.ParentNumber != "5.3" {
			t.Errorf("parent: want 5.3, got %q", sec.ParentNumber)
		}
	})

	t.Run("get section not cached", func(t *testing.T) {
		_, err := store.GetSection("99.999", "Rel-18", "1")
		if !errors.Is(err, ErrNotCached) {
			t.Errorf("expected ErrNotCached, got %v", err)
		}
	})

	t.Run("list children root", func(t *testing.T) {
		children, err := store.ListChildSections("38.331", "Rel-18", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(children) != 1 {
			t.Errorf("root children: want 1, got %d", len(children))
		}
		if children[0].SectionNumber != "5" {
			t.Errorf("root child: want 5, got %s", children[0].SectionNumber)
		}
	})

	t.Run("list children", func(t *testing.T) {
		children, err := store.ListChildSections("38.331", "Rel-18", "5")
		if err != nil {
			t.Fatal(err)
		}
		if len(children) != 1 {
			t.Errorf("children of 5: want 1, got %d", len(children))
		}
		if children[0].SectionNumber != "5.3" {
			t.Errorf("child: want 5.3, got %s", children[0].SectionNumber)
		}
	})
}

func TestSpecStore_Delete(t *testing.T) {
	db, _ := openTestStoreDB(t)
	defer db.Close()

	store := NewSpecStore(db)

	sections := []model.Section{
		{SectionNumber: "1", Title: "Test", Content: "content"},
	}
	store.InsertSections("38.331", "Rel-18", sections)

	store.DeleteContent("38.331")

	has, _ := store.HasContent("38.331", "Rel-18")
	if has {
		t.Error("expected HasContent=false after delete")
	}
}

func TestSpecStore_CachedSpecs(t *testing.T) {
	db, _ := openTestStoreDB(t)
	defer db.Close()

	store := NewSpecStore(db)

	store.InsertSections("38.331", "Rel-18", []model.Section{
		{SectionNumber: "1", Title: "Test", Content: "c"},
	})
	store.InsertSections("23.501", "Rel-18", []model.Section{
		{SectionNumber: "1", Title: "Test", Content: "c"},
	})

	cached, err := store.ListCachedSpecs()
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 2 {
		t.Errorf("cached specs: want 2, got %d", len(cached))
	}
}
