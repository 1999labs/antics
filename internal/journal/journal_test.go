package journal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// useTempDir points the journal at a throwaway directory for the duration of a
// test, so we never touch the real ~/.antics.
func useTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := testDir
	testDir = dir
	t.Cleanup(func() { testDir = old })
	return dir
}

func TestActionRoundTrip(t *testing.T) {
	d := onDisk{PID: 42, Records: map[string][]Action{
		"0": {Delete("/tmp/x")},
		"1": {Exec("pkill", "-CONT", "-f", "svc")},
	}}
	blob, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var got onDisk
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatal(err)
	}
	if got.Records["0"][0].Op != OpDelete || got.Records["0"][0].Path != "/tmp/x" {
		t.Errorf("delete action did not round-trip: %+v", got.Records["0"])
	}
	if got.Records["1"][0].Op != OpExec || len(got.Records["1"][0].Argv) != 4 {
		t.Errorf("exec action did not round-trip: %+v", got.Records["1"])
	}
}

func TestBeginIsLazy(t *testing.T) {
	dir := useTempDir(t)
	j := Begin()
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("Begin wrote files before any Record: %v", entries)
	}
	_ = j
}

func TestRecordCreatesFileThenClearAndFinishDeleteIt(t *testing.T) {
	dir := useTempDir(t)
	j := Begin()
	if err := j.Record("0", Delete("/tmp/whatever")); err != nil {
		t.Fatal(err)
	}
	if !fileExists(j.path) {
		t.Fatal("journal file missing after Record")
	}
	// No leftover temp files from the atomic write.
	for _, e := range mustReadDir(t, dir) {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
	if err := j.Clear("0"); err != nil {
		t.Fatal(err)
	}
	if err := j.Finish(); err != nil {
		t.Fatal(err)
	}
	if fileExists(j.path) {
		t.Error("journal file should be gone after Clear + Finish (clean run)")
	}
}

func TestFinishKeepsFileWhenRecordsRemain(t *testing.T) {
	useTempDir(t)
	j := Begin()
	if err := j.Record("0", Delete("/tmp/whatever")); err != nil {
		t.Fatal(err)
	}
	// A failed Restore would NOT Clear the key; Finish must leave the file.
	if err := j.Finish(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(j.path) {
		t.Error("journal file should remain when records are uncleared (failed restore)")
	}
}

func TestRecoverReplaysDeadPIDDelete(t *testing.T) {
	dir := useTempDir(t)
	// A target file the crashed run was supposed to delete.
	target := filepath.Join(t.TempDir(), "leftover.junk")
	if err := os.WriteFile(target, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadPID := spawnAndReap(t)
	writeJournal(t, dir, deadPID, map[string][]Action{"0": {Delete(target)}})

	notes, err := Recover()
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if len(notes) == 0 {
		t.Error("expected recovery notes")
	}
	if fileExists(target) {
		t.Error("Recover did not delete the leftover target file")
	}
	if fileExists(filepath.Join(dir, journalName(deadPID))) {
		t.Error("Recover did not remove the replayed journal")
	}
}

func TestRecoverSkipsAlivePID(t *testing.T) {
	dir := useTempDir(t)
	// A journal owned by THIS (alive) process must be left untouched.
	writeJournal(t, dir, os.Getpid(), map[string][]Action{"0": {Delete("/tmp/nope")}})
	if _, err := Recover(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(dir, journalName(os.Getpid()))) {
		t.Error("Recover removed a live process's journal")
	}
}

func TestRecoverMissingDirIsNoError(t *testing.T) {
	testDir = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(func() { testDir = "" })
	notes, err := Recover()
	if err != nil || notes != nil {
		t.Errorf("missing dir: notes=%v err=%v", notes, err)
	}
}

func TestParsePIDFromFilename(t *testing.T) {
	ok := map[string]int{"journal-1234-abc.json": 1234, "journal-1-deadbeef.json": 1}
	for name, want := range ok {
		if got, valid := pidFromName(name); !valid || got != want {
			t.Errorf("pidFromName(%q) = %d,%v want %d,true", name, got, valid, want)
		}
	}
	for _, name := range []string{"journal-1.json", "journal--x.json", "journal-.json", "journal-abc-x.json", "notajournal.txt", "journal-0-x.json"} {
		if _, valid := pidFromName(name); valid {
			t.Errorf("pidFromName(%q) accepted a bad name", name)
		}
	}
}

func TestAlivePIDCheck(t *testing.T) {
	if !alive(os.Getpid()) {
		t.Error("alive(self) = false")
	}
	if alive(0) {
		t.Error("alive(0) = true")
	}
	if alive(spawnAndReap(t)) {
		t.Error("alive(reaped pid) = true")
	}
}

// --- helpers ---

func spawnAndReap(t *testing.T) int {
	t.Helper()
	c := exec.Command("true")
	if err := c.Run(); err != nil {
		// Fall back to a sleep that we kill, if "true" is unavailable.
		t.Skipf("could not spawn a short-lived process: %v", err)
	}
	return c.Process.Pid // exited, so this pid is dead (reuse is improbable mid-test)
}

func writeJournal(t *testing.T, dir string, pid int, records map[string][]Action) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	d := onDisk{PID: pid, Records: records}
	blob, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, journalName(pid)), blob, 0o644); err != nil {
		t.Fatal(err)
	}
}

func journalName(pid int) string { return "journal-" + strconv.Itoa(pid) + "-test.json" }

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mustReadDir(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	e, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
