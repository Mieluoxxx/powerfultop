package icons

import "testing"

func TestEveryIconParses(t *testing.T) {
	names := Names()
	if len(names) < 50 {
		t.Fatalf("only %d icons embedded", len(names))
	}
	for _, name := range names {
		if Get(name) == nil {
			t.Errorf("%s did not parse", name)
		}
	}
}
