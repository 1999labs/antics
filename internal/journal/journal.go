// Package journal is Antics' crash-recovery safety net. Most antics undo
// themselves on teardown — even on Ctrl-C — but a HARD kill (SIGKILL, an OOM,
// a power loss) skips that deferred cleanup entirely. For antics that change
// state OUTSIDE this process (a junk file on disk, a frozen process, an OS
// network rule), that would leave the mess behind: exactly what Antics promises
// never to do.
//
// So before such an antic runs, the runner records an antic-agnostic undo
// action here. A clean run clears each record as its Restore succeeds and
// deletes the journal at the end, so nothing survives. But if the process is
// hard-killed, the journal file outlives it — and the next `antics run` (or an
// explicit `antics restore`) replays the leftover actions, finishing the
// cleanup the dead run never reached.
package journal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Op identifies what an Action does. Kept tiny and antic-agnostic so the
// journal never needs to know about specific antics.
type Op string

const (
	// OpDelete removes Path from disk (a file or directory).
	OpDelete Op = "delete"
	// OpExec runs Argv (argv[0] is the program).
	OpExec Op = "exec"
)

// Action is a single undo step, replayable WITHOUT a live antic instance —
// everything it needs is serialized here, because after a crash the on-disk
// record is all that survives.
type Action struct {
	Op   Op       `json:"op"`
	Path string   `json:"path,omitempty"`
	Argv []string `json:"argv,omitempty"`
}

// Delete builds an action that removes a file or directory.
func Delete(path string) Action { return Action{Op: OpDelete, Path: path} }

// Exec builds an action that runs a command. argv[0] is the program.
func Exec(argv ...string) Action { return Action{Op: OpExec, Argv: argv} }

// onDisk is one run's journal as persisted to disk.
type onDisk struct {
	PID     int                 `json:"pid"`
	Started string              `json:"started"`
	Records map[string][]Action `json:"records"`
}

// testDir, when set, redirects the journal location (used by tests).
var testDir string

func journalDir() string {
	if testDir != "" {
		return testDir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".antics", "journal")
	}
	return filepath.Join(os.TempDir(), "antics", "journal")
}

// Journal is one run's handle. It is lazy: no file touches disk until the first
// Record, so a run whose antics need no recovery never writes anything.
type Journal struct {
	mu      sync.Mutex
	dir     string
	path    string
	data    onDisk
	written bool
}

// Begin starts a journal for the current process. It never fails and writes
// nothing yet.
func Begin() *Journal {
	dir := journalDir()
	pid := os.Getpid()
	return &Journal{
		dir:  dir,
		path: filepath.Join(dir, fmt.Sprintf("journal-%d-%s.json", pid, token())),
		data: onDisk{
			PID:     pid,
			Started: time.Now().UTC().Format(time.RFC3339),
			Records: map[string][]Action{},
		},
	}
}

// Record persists the undo actions for one antic under a stable key. The first
// call materializes the journal file. Re-recording the same key overwrites it,
// so it's safe to call again once an antic finalizes its undo at Commit time.
func (j *Journal) Record(key string, actions ...Action) error {
	if len(actions) == 0 {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.data.Records[key] = actions
	return j.flushLocked()
}

// Clear drops one antic's record after its Restore succeeded. If that empties
// the journal, the file is removed.
func (j *Journal) Clear(key string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.data.Records[key]; !ok {
		return nil
	}
	delete(j.data.Records, key)
	if len(j.data.Records) == 0 {
		return j.removeLocked()
	}
	return j.flushLocked()
}

// Finish ends a run. If every record was cleared (a clean run), the journal
// file is deleted. If any remain — an antic whose Restore failed — the file is
// left in place so a future run can finish the cleanup.
func (j *Journal) Finish() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.data.Records) == 0 {
		return j.removeLocked()
	}
	return nil
}

func (j *Journal) flushLocked() error {
	if err := os.MkdirAll(j.dir, 0o700); err != nil {
		return err
	}
	blob, err := json.MarshalIndent(j.data, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temp file in the SAME directory and rename, so a reader never
	// sees a half-written journal (and the rename stays on one filesystem).
	tmp, err := os.CreateTemp(j.dir, fmt.Sprintf("journal-%d-*.tmp", j.data.PID))
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(blob); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, j.path); err != nil {
		os.Remove(name)
		return err
	}
	// Best-effort: fsync the directory so the rename survives a power loss (the
	// crash mode this journal exists for). A failure here isn't fatal.
	if d, err := os.Open(j.dir); err == nil {
		d.Sync()
		d.Close()
	}
	j.written = true
	return nil
}

// token returns a short random hex string that makes each run's journal
// filename unique, so a recycled PID can't clobber a prior crashed run's
// still-uncleared journal before a later run gets to replay it.
func token() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0"
	}
	return hex.EncodeToString(b[:])
}

func (j *Journal) removeLocked() error {
	if !j.written {
		return nil
	}
	err := os.Remove(j.path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	j.written = false
	return nil
}

// Recover finishes the cleanup of any run that was hard-killed before its
// teardown ran. It scans the journal directory and, for each journal whose
// owning process is no longer alive (and isn't this one), replays the recorded
// undo actions and deletes the journal. It returns human-readable notes about
// what it cleaned up. A missing directory is not an error.
func Recover() ([]string, error) {
	dir := journalDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	self := os.Getpid()
	var notes []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		pid, ok := pidFromName(e.Name())
		if !ok {
			continue // not a journal-<pid>.json file
		}
		if pid == self || alive(pid) {
			continue // our own journal, or a still-running antics process
		}
		notes = append(notes, replayFile(filepath.Join(dir, e.Name()), pid)...)
	}
	return notes, nil
}

func replayFile(path string, pid int) []string {
	blob, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("couldn't read leftover journal %s: %v", path, err)}
	}
	var d onDisk
	if err := json.Unmarshal(blob, &d); err != nil {
		return []string{fmt.Sprintf("leftover journal %s is corrupt; leaving it in place: %v", path, err)}
	}

	// Undo the most-recent antic first: sort record keys numerically descending,
	// mirroring the runner's reverse-order teardown.
	keys := make([]string, 0, len(d.Records))
	for k := range d.Records {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool { return atoi(keys[a]) > atoi(keys[b]) })

	var notes []string
	allOK := true
	for _, k := range keys {
		for _, act := range d.Records[k] {
			if err := apply(act); err != nil {
				allOK = false
				notes = append(notes, fmt.Sprintf("crash-recovery for pid %d: couldn't %s: %v", pid, describe(act), err))
			} else {
				notes = append(notes, fmt.Sprintf("crash-recovery for pid %d: %s", pid, describe(act)))
			}
		}
	}
	if allOK {
		os.Remove(path)
	} else {
		notes = append(notes, fmt.Sprintf("left %s in place; run `antics restore` to retry", path))
	}
	return notes
}

func apply(a Action) error {
	switch a.Op {
	case OpDelete:
		if a.Path == "" {
			return nil
		}
		return os.RemoveAll(a.Path) // nil if already gone — idempotent
	case OpExec:
		if len(a.Argv) == 0 {
			return nil
		}
		_, err := exec.Command(a.Argv[0], a.Argv[1:]...).CombinedOutput()
		if err != nil {
			// Ran but exited nonzero: the mess was likely already cleared (an
			// idempotent undo on a clean machine) or a retry won't fix it —
			// either way, don't strand the journal forever.
			if _, ok := err.(*exec.ExitError); ok {
				return nil
			}
			// Couldn't even start it (missing binary, etc.): keep the journal
			// so a later run can retry.
			return err
		}
		return nil
	default:
		return nil // unknown op from another binary version: skip, don't strand
	}
}

func describe(a Action) string {
	switch a.Op {
	case OpDelete:
		return "removed leftover file " + a.Path
	case OpExec:
		return "ran " + strings.Join(a.Argv, " ")
	default:
		return string(a.Op)
	}
}

// pidFromName extracts the pid from "journal-<pid>-<token>.json", rejecting
// names that don't fit (a stray .tmp, a missing token, a non-numeric pid).
func pidFromName(name string) (int, bool) {
	if !strings.HasPrefix(name, "journal-") || !strings.HasSuffix(name, ".json") {
		return 0, false
	}
	mid := name[len("journal-") : len(name)-len(".json")] // "<pid>-<token>"
	dash := strings.IndexByte(mid, '-')
	if dash <= 0 {
		return 0, false
	}
	pid, err := strconv.Atoi(mid[:dash])
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// alive reports whether a process with the given pid currently exists. A signal
// of 0 performs the kernel's permission/existence check without delivering a
// signal; EPERM means the process exists but isn't ours.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
