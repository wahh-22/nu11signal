package history

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var (
	_ Recents = (*File)(nil)
	_ Recents = (*Memory)(nil)
)

func TestPush(t *testing.T) {
	tests := []struct {
		name string
		list []string
		term string
		want []string
	}{
		{"into empty", nil, "daft punk", []string{"daft punk"}},
		{"most recent first", []string{"a"}, "b", []string{"b", "a"}},
		{"trims", []string{"a"}, "  b  ", []string{"b", "a"}},
		{"blank is ignored", []string{"a"}, "   ", []string{"a"}},
		{"duplicate moves to front", []string{"a", "b", "c"}, "c", []string{"c", "a", "b"}},
		{"dedupe ignores case", []string{"a", "Daft Punk"}, "DAFT PUNK", []string{"DAFT PUNK", "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Push(tt.list, tt.term); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Push(%q, %q) = %q; want %q", tt.list, tt.term, got, tt.want)
			}
		})
	}
}

func TestPushCapsAndLeavesInputAlone(t *testing.T) {
	var list []string
	for i := range Max + 3 {
		list = Push(list, fmt.Sprint(i))
	}
	if len(list) != Max || list[0] != fmt.Sprint(Max+2) {
		t.Fatalf("list = %q; want the %d most recent, newest first", list, Max)
	}
	before := append([]string(nil), list...)
	_ = Push(list, "new")
	if !reflect.DeepEqual(list, before) {
		t.Fatalf("Push modified its input: %q", list)
	}
}

func TestFilePersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "recent.json")
	f := NewFile(path)
	for _, term := range []string{"queen", "daft punk", "Queen"} {
		if err := f.Add(term); err != nil {
			t.Fatalf("Add(%q): %v", term, err)
		}
	}
	got, err := NewFile(path).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{"Queen", "daft punk"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %q; want %q", got, want)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries; want only recent.json (no temp files left)", len(entries))
	}
}

func TestFileMissingIsEmpty(t *testing.T) {
	got, err := NewFile(filepath.Join(t.TempDir(), "recent.json")).Load()
	if err != nil || len(got) != 0 {
		t.Fatalf("Load = %q, %v; want empty, nil", got, err)
	}
}

func TestFileCorruptIsEmptyAndRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := NewFile(path)
	got, err := f.Load()
	if err != nil || len(got) != 0 {
		t.Fatalf("Load = %q, %v; want empty, nil", got, err)
	}
	if err := f.Add("queen"); err != nil {
		t.Fatalf("Add over a corrupt file: %v", err)
	}
	if got, _ := f.Load(); !reflect.DeepEqual(got, []string{"queen"}) {
		t.Fatalf("Load after Add = %q; want [queen]", got)
	}
}

func TestFileNormalizesHandEditedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	content := `{"terms":["a"," A ","","b","c","d","e","f","g","h","i","j","k","l"]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewFile(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %q; want %q", got, want)
	}
}

func TestFileWriteFailureIsReported(t *testing.T) {
	// The parent "directory" is a regular file, so it cannot be created.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewFile(filepath.Join(blocker, "recent.json")).Add("queen"); err == nil {
		t.Fatal("Add succeeded where the directory cannot exist")
	}
}

func TestMemory(t *testing.T) {
	m := NewMemory()
	if got, err := m.Load(); err != nil || len(got) != 0 {
		t.Fatalf("Load = %q, %v; want empty", got, err)
	}
	_ = m.Add("a")
	_ = m.Add("b")
	got, _ := m.Load()
	if want := []string{"b", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %q; want %q", got, want)
	}
	got[0] = "mutated"
	if again, _ := m.Load(); again[0] != "b" {
		t.Fatal("Load exposed internal state")
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "recent.json" || filepath.Base(filepath.Dir(path)) != "nu11signal" {
		t.Fatalf("DefaultPath = %q; want .../nu11signal/recent.json", path)
	}
}
