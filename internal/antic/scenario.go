package antic

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Scenario is an ordered list of antics to run together — a "battery" of
// mischief. It's loaded from a simple, dependency-free config file so Antics
// stays a single static binary with nothing to install.
type Scenario struct {
	Name   string
	Antics []Antic
}

// LoadScenario parses an antics scenario file.
//
// The format is deliberately tiny so we need no YAML dependency (which keeps
// the binary self-contained). Example:
//
//	name: api-meltdown
//
//	[kill]
//	match: my-api
//	every: 5s
//
//	[diskfill]
//	megabytes: 500
//
//	[cpuhog]
//	cores: 2
//
// A line `[antic-name]` starts a new antic block. `key: value` lines inside it
// become that antic's params. Blank lines and `#` comments are ignored.
func LoadScenario(path string) (*Scenario, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening scenario: %w", err)
	}
	defer f.Close()

	sc := &Scenario{Name: "untitled"}

	var (
		curName   string
		curParams map[string]any
	)
	flush := func() error {
		if curName == "" {
			return nil
		}
		a, err := Build(curName, curParams)
		if err != nil {
			return err
		}
		sc.Antics = append(sc.Antics, a)
		curName, curParams = "", nil
		return nil
	}

	s := bufio.NewScanner(f)
	lineNo := 0
	for s.Scan() {
		lineNo++
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Top-level scenario name.
		if strings.HasPrefix(line, "name:") && curName == "" {
			sc.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			continue
		}
		// New antic block.
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if err := flush(); err != nil {
				return nil, err
			}
			curName = strings.TrimSpace(line[1 : len(line)-1])
			curParams = map[string]any{}
			continue
		}
		// key: value inside an antic block.
		if curName != "" {
			k, v, ok := strings.Cut(line, ":")
			if !ok {
				return nil, fmt.Errorf("line %d: expected `key: value`, got %q", lineNo, line)
			}
			curParams[strings.TrimSpace(k)] = coerce(strings.TrimSpace(v))
			continue
		}
		return nil, fmt.Errorf("line %d: %q is outside any [antic] block", lineNo, line)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(sc.Antics) == 0 {
		return nil, fmt.Errorf("scenario %q has no antics in it", path)
	}
	return sc, nil
}

// coerce turns a string value into an int when it looks like one, so params
// like `megabytes: 500` arrive as ints, while `match: my-api` stays a string.
func coerce(v string) any {
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return v
}
