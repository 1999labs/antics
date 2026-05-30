package antic

import "testing"

func TestParamStringAcceptsInts(t *testing.T) {
	s, err := paramString(map[string]any{"k": 8080}, "k")
	if err != nil {
		t.Fatalf("paramString int: %v", err)
	}
	if s != "8080" {
		t.Errorf("got %q, want 8080", s)
	}
}

func TestParamStringMissing(t *testing.T) {
	if _, err := paramString(map[string]any{}, "k"); err == nil {
		t.Error("expected error for missing key")
	}
}

func TestParamDuration(t *testing.T) {
	d, err := paramDuration(map[string]any{"every": "5s"}, "every", 0)
	if err != nil {
		t.Fatal(err)
	}
	if d.String() != "5s" {
		t.Errorf("got %s, want 5s", d)
	}
	d2, err := paramDuration(map[string]any{}, "every", 0)
	if err != nil || d2 != 0 {
		t.Errorf("default duration: d=%v err=%v", d2, err)
	}
	if _, err := paramDuration(map[string]any{"every": "notaduration"}, "every", 0); err == nil {
		t.Error("expected error for bad duration string")
	}
}

func TestParamInt(t *testing.T) {
	if got := paramInt(map[string]any{"n": 3}, "n", 1); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
	if got := paramInt(map[string]any{}, "n", 7); got != 7 {
		t.Errorf("default: got %d, want 7", got)
	}
}

func TestBuildUnknownAntic(t *testing.T) {
	if _, err := Build("nope", nil); err == nil {
		t.Error("expected error for unknown antic")
	}
}

func TestAvailableIsSortedAndComplete(t *testing.T) {
	names := Available()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("Available not sorted: %v", names)
		}
	}
	if len(names) < 7 {
		t.Errorf("expected at least the 7 portable antics, got %v", names)
	}
}
