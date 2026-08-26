package git

import "testing"

func FuzzValidateRebaseTodo(f *testing.F) {
	f.Add("pick 0123456789012345678901234567890123456789 subject\n")
	f.Add("exec echo hello\n")
	f.Add("# comment\nnoop\n")
	f.Fuzz(func(t *testing.T, todo string) {
		if len(todo) > 64<<10 {
			t.Skip()
		}
		err := ValidateRebaseTodo(todo)
		if err == nil {
			if err := ValidateRebaseTodo(todo + "\n# fuzz regression comment\n"); err != nil {
				t.Fatalf("adding a comment invalidated accepted todo: %v", err)
			}
		}
	})
}
