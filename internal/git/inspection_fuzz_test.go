package git

import (
	"reflect"
	"testing"
)

func FuzzParseInspectionLog(f *testing.F) {
	f.Add([]byte("\x1e0123456789012345678901234567890123456789\x001234567\x00\x00HEAD -> main\x00subject\x00A U Thor\x00a@example.com\x002026-01-02T03:04:05Z\x002026-01-02T03:04:06Z\x00"))
	f.Add([]byte("graph \x1eincomplete"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		got, incomplete, err := parseInspectionLog(data)
		got2, incomplete2, err2 := parseInspectionLog(data)
		if (err == nil) != (err2 == nil) || incomplete != incomplete2 || !reflect.DeepEqual(got, got2) {
			t.Fatal("log parser is not deterministic")
		}
		if len(got) > len(data) {
			t.Fatalf("parsed %d entries from %d bytes", len(got), len(data))
		}
	})
}

func FuzzParseInspectionRefs(f *testing.F) {
	f.Add([]byte("\x00*\x00refs/heads/main\x00main\x000123456789012345678901234567890123456789\x00\x00origin/main\x00origin/main\x00\x00subject\x00"))
	f.Add([]byte("\x00\x00refs/remotes/origin/HEAD\x00origin/HEAD\x00id\x00\x00\x00\x00refs/remotes/origin/main\x00alias\x00"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		refs, incomplete, err := parseInspectionRefs(data)
		refs2, incomplete2, err2 := parseInspectionRefs(data)
		if (err == nil) != (err2 == nil) || incomplete != incomplete2 || !reflect.DeepEqual(refs, refs2) {
			t.Fatal("refs parser is not deterministic")
		}
		for _, ref := range refs {
			if ref.Kind != RefLocal && ref.Kind != RefRemote && ref.Kind != RefTag {
				t.Fatalf("invalid ref kind %v", ref.Kind)
			}
		}
	})
}

func FuzzParseInspectionState(f *testing.F) {
	f.Add("pick 0123456789012345678901234567890123456789 subject\n")
	f.Add("revert 0123456789012345678901234567890123456789 x\npick fedcba9876543210fedcba9876543210fedcba98 y\n")
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 64<<10 {
			t.Skip()
		}
		ids, err := parseAdminOIDs(text)
		if err == nil {
			for _, id := range ids {
				if !isHexOID(id) {
					t.Fatalf("accepted invalid object ID %q", id)
				}
			}
		}
		kind, heads := parseSequencerTodo(text)
		if kind != 0 && kind != OperationCherryPick && kind != OperationRevert {
			t.Fatalf("invalid operation kind %v", kind)
		}
		for _, id := range heads {
			if !isHexOID(id) {
				t.Fatalf("sequencer returned invalid object ID %q", id)
			}
		}
	})
}
