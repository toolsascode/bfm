package registry

import (
	"testing"

	"github.com/toolsascode/bfm/api/internal/backends"
)

func TestParseTagFilter(t *testing.T) {
	m, err := ParseTagFilter([]string{"env=prod", " feature=billing "})
	if err != nil {
		t.Fatal(err)
	}
	if m["env"] != "prod" || m["feature"] != "billing" {
		t.Fatalf("map = %#v", m)
	}
	_, err = ParseTagFilter([]string{"not-a-tag"})
	if err == nil {
		t.Fatal("expected error")
	}
	m2, err := ParseTagFilter([]string{"a=b", "a=c"})
	if err != nil {
		t.Fatal(err)
	}
	if m2["a"] != "c" {
		t.Fatalf("last wins: %#v", m2)
	}
}

func TestParseTagFilter_ValueWithEquals(t *testing.T) {
	m, err := ParseTagFilter([]string{"k=v=2"})
	if err != nil {
		t.Fatal(err)
	}
	if m["k"] != "v=2" {
		t.Fatalf("got %q", m["k"])
	}
}

func TestInMemoryRegistry_FindByTarget_Tags(t *testing.T) {
	reg := NewInMemoryRegistry()
	_ = reg.Register(&backends.MigrationScript{
		Version: "20240101120000", Name: "a", Connection: "c", Backend: "postgresql",
		Tags: []string{"env=prod", "tier=gold"},
	})
	_ = reg.Register(&backends.MigrationScript{
		Version: "20240101120001", Name: "b", Connection: "c", Backend: "postgresql",
		Tags: []string{"env=staging"},
	})
	_ = reg.Register(&backends.MigrationScript{
		Version: "20240101120002", Name: "c", Connection: "c", Backend: "postgresql",
	})

	target := &MigrationTarget{Connection: "c", Tags: []string{"env=prod", "tier=gold"}}
	got, err := reg.FindByTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("got %d migrations, first name=%v", len(got), got)
	}

	got2, err := reg.FindByTarget(&MigrationTarget{Connection: "c", Tags: []string{"env=prod"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 1 {
		t.Fatalf("want 1, got %d", len(got2))
	}

	got3, err := reg.FindByTarget(&MigrationTarget{Connection: "c", Tags: []string{"env=prod", "tier=silver"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got3) != 0 {
		t.Fatalf("want 0, got %d", len(got3))
	}

	_, err = reg.FindByTarget(&MigrationTarget{Connection: "c", Tags: []string{"bad"}})
	if err == nil {
		t.Fatal("expected invalid tag error")
	}
}
