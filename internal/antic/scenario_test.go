package antic

import (
	"os"
	"path/filepath"
	"testing"
)

func writeScenario(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.antics")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadScenarioParsesNameAndAntics(t *testing.T) {
	p := writeScenario(t, `# a leading comment
name: demo

[cpuhog]
cores: 2

[memhog]
megabytes: 64
`)
	sc, err := LoadScenario(p)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	if sc.Name != "demo" {
		t.Errorf("name = %q, want demo", sc.Name)
	}
	if len(sc.Antics) != 2 {
		t.Fatalf("got %d antics, want 2", len(sc.Antics))
	}
	if got := sc.Antics[0].Name(); got != "cpuhog" {
		t.Errorf("antic[0] = %q, want cpuhog", got)
	}
	if got := sc.Antics[1].Name(); got != "memhog" {
		t.Errorf("antic[1] = %q, want memhog", got)
	}
}

func TestLoadScenarioErrors(t *testing.T) {
	cases := map[string]string{
		"no antics":           "name: nothing\n",
		"unknown antic":       "[nope]\nfoo: bar\n",
		"key outside block":   "name: x\nstray: value\n",
		"malformed key:value": "[cpuhog]\nnotakeyvalue\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := writeScenario(t, body)
			if _, err := LoadScenario(p); err == nil {
				t.Errorf("expected error for %q, got nil", name)
			}
		})
	}
}

func TestLoadScenarioMissingFile(t *testing.T) {
	if _, err := LoadScenario(filepath.Join(t.TempDir(), "nope.antics")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCoerceIntsButNotStrings(t *testing.T) {
	if v := coerce("500"); v != 500 {
		t.Errorf("coerce(\"500\") = %v (%T), want int 500", v, v)
	}
	if v := coerce("my-api"); v != "my-api" {
		t.Errorf("coerce(\"my-api\") = %v, want string", v)
	}
}
