package git

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzValidateRepoRelative(f *testing.F) {
	f.Add("dir/file", false)
	f.Add("../escape", true)
	f.Add(".", true)
	f.Fuzz(func(t *testing.T, path string, allowDot bool) {
		if len(path) > 4096 {
			t.Skip()
		}
		clean, err := validateRepoRelative(path, allowDot)
		if err != nil {
			return
		}
		if clean != filepath.Clean(filepath.FromSlash(path)) || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			t.Fatalf("unsafe clean path %q from %q", clean, path)
		}
		if again, err := validateRepoRelative(clean, allowDot); err != nil || again != clean {
			t.Fatalf("validation is not idempotent: %q, %v", again, err)
		}
	})
}
