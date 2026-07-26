package store

import (
	"errors"
	"sort"
	"strings"
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

func TestSortSectionKey(t *testing.T) {
	tests := []struct {
		input []string
		want  []string
	}{
		{
			input: []string{"6.1.10", "6.1.2", "6.1.1"},
			want:  []string{"6.1.1", "6.1.2", "6.1.10"},
		},
		{
			input: []string{"10", "1", "2", "3"},
			want:  []string{"1", "2", "3", "10"},
		},
		{
			input: []string{"A.2", "A.1", "A.10"},
			want:  []string{"A.1", "A.2", "A.10"},
		},
		{
			input: []string{"5.1.1", "5.1", "5"},
			want:  []string{"5", "5.1", "5.1.1"},
		},
		{
			input: []string{"Foreword", "1", "A"},
			want:  []string{"1", "A", "Foreword"},
		},
		{
			input: []string{"B.1", "A.2", "A.1.3", "A.1"},
			want:  []string{"A.1", "A.1.3", "A.2", "B.1"},
		},
	}

	for _, tt := range tests {
		name := strings.Join(tt.input, ",")
		t.Run(name, func(t *testing.T) {
			got := make([]string, len(tt.input))
			copy(got, tt.input)
			sort.Slice(got, func(i, j int) bool {
				return sortSectionKey(got[i]) < sortSectionKey(got[j])
			})
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("at %d: want %q, got %q", i, tt.want[i], got[i])
				}
			}
		})
	}
}

func TestSpecStore_NaturalSortOrder(t *testing.T) {
	db, _ := openTestStoreDB(t)
	defer db.Close()

	store := NewSpecStore(db)

	sections := []model.Section{
		{SectionNumber: "6.1.10", ParentNumber: "6.1", Title: "tenth", Content: "x"},
		{SectionNumber: "6.1.2", ParentNumber: "6.1", Title: "second", Content: "x"},
		{SectionNumber: "6.1.1", ParentNumber: "6.1", Title: "first", Content: "x"},
		{SectionNumber: "5", ParentNumber: "", Title: "five", Content: "x"},
		{SectionNumber: "6", ParentNumber: "", Title: "six", Content: "x"},
		{SectionNumber: "6.1", ParentNumber: "6", Title: "overview", Content: "x"},
	}
	store.InsertSections("99.999", "Rel-99", sections)

	t.Run("root order", func(t *testing.T) {
		children, err := store.ListChildSections("99.999", "Rel-99", "")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"5", "6"}
		for i, w := range want {
			if i >= len(children) || children[i].SectionNumber != w {
				t.Errorf("root[%d]: want %q, got %q", i, w, children[i].SectionNumber)
			}
		}
	})

	t.Run("child numeric order", func(t *testing.T) {
		children, err := store.ListChildSections("99.999", "Rel-99", "6.1")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"6.1.1", "6.1.2", "6.1.10"}
		for i, w := range want {
			if i >= len(children) || children[i].SectionNumber != w {
				t.Errorf("child[%d]: want %q, got %q", i, w, children[i].SectionNumber)
			}
		}
	})
}
