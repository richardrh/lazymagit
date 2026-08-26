package git

import (
	"reflect"
	"testing"
)

func TestParsePorcelainV2StatusRecords(t *testing.T) {
	out := []byte("1 M. N... 100644 100644 100644 aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb staged name\x00" +
		"2 R. N... 100644 100644 100644 cccccccccccccccccccccccccccccccccccccccc dddddddddddddddddddddddddddddddddddddddd R100 renamed name\x00old name\x00" +
		"? untracked name\x00")
	got, err := parsePorcelainV2Status(out)
	if err != nil {
		t.Fatal(err)
	}
	want := Status{Files: []FileStatus{
		{Path: "staged name", Staged: ChangeModified},
		{Path: "renamed name", OriginalPath: "old name", Staged: ChangeRenamed},
		{Path: "untracked name", Unstaged: ChangeUntracked},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status = %#v, want %#v", got, want)
	}
	if _, err := parsePorcelainV2Status([]byte("2 R. N... 100644 100644 100644 a b R100 path")); err == nil {
		t.Fatal("rename without original path was accepted")
	}
}

func FuzzParsePorcelainV2Status(f *testing.F) {
	f.Add([]byte("? file with spaces\x00"))
	f.Add([]byte("1 .M N... 100644 100644 100644 a b path\x00"))
	f.Add([]byte("2 R. N... 100644 100644 100644 a b R100 new\x00old\x00"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		got, err := parsePorcelainV2Status(data)
		got2, err2 := parsePorcelainV2Status(data)
		if (err == nil) != (err2 == nil) || !reflect.DeepEqual(got, got2) {
			t.Fatal("status parser is not deterministic")
		}
		if len(got.Files) > len(data) {
			t.Fatalf("parsed %d files from %d bytes", len(got.Files), len(data))
		}
	})
}
