// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The connection guard's hook against the driver's Open (master CI, job
// 111449911669): modernc.org/sqlite requires every registration to happen
// before the first call to Open (driver.go, the Driver type's documentation),
// and its Open reads the hook list without the registration mutex. A hook
// registered at the first ledger open of the process is a write that no
// conversation store opening its connections in another goroutine is ordered
// against: the race detector names RegisterConnectionHook and Driver.Open.
//
// The first ledger open has to be the first of its PROCESS — the registration
// it raced was guarded by a process-wide sync.Once — so every row runs in
// CHILD processes (this test binary re-executed) that do nothing else.
// Evidence level: compiled test binary in a separate OS process, under -race.

package sqlite

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	convsqlite "github.com/Sebastian197/korvun/internal/conversation/sqlite"
)

// raceChildEnv carries the door the child opens the ledger through.
const raceChildEnv = "KORVUN_REGISTER_GUARD_RACE_CHILD"

// raceOpeners is how many goroutines open conversation stores, raceChildren
// how many child processes each door runs, and raceDeadline the cap on the
// wait for the openers' witnesses and on how long the openers keep opening:
// they stop once the ledger open has returned, which the ledger's goroutine
// announces with a directory of its own.
//
// The interleaving the CI saw is a conversation open that READ the hook list
// before the first ledger open WROTE it, with nothing ordering the two: the
// ledger's goroutine touches the driver for the first time at that write.
// Each opener, once its first convsqlite.Open has returned, creates a witness
// directory: that Open ran statements on a connection, so a Driver.Open had
// returned, and its read of the hook list with it. The oracle is the count of
// witnesses the child observes immediately before the ledger open begins; the
// parent demands all of them.
//
// A witness proves the read came first, not that nothing orders it before the
// write; the mould's own machinery must add no such order. Between an
// opener's first open and the ledger open it uses only os.Mkdir, os.Stat,
// time.Sleep, strconv and filepath: no os.WriteFile, whose syscall.Write
// releases the race detector's ioSync, and no fmt.Sprintf, whose sync.Pool
// releases and acquires on every Put and Get. The probe row checks that claim
// by impossibility, opener by opener, instead of trusting it.
//
// The detector does not promise to see every race it is shown: with its
// default history it can lose the stack of the earlier access — in a ledger
// door, Driver.Open's read of the hook list — and stay silent. A
// ledger door therefore runs several children and one report is red. In the
// probe child each opener stops opening after its first open and parks on a
// channel the ledger's goroutine closes only after its read, which orders
// nothing before that read: a goroutine that exits can hand its place in
// the detector to a new one and the stack of its write is lost. The child
// runs with raceProbeGORACE, the full history and no report suppression;
// there every unordered opener is one report. The probe row certifies the
// wait; what an opener does after its first open is not in it.
const (
	raceOpeners  = 8
	raceChildren = 4
	raceDeadline = 60 * time.Second
)

// raceProbeGORACE is the detector configuration of the probe child.
const raceProbeGORACE = "GORACE=history_size=7 suppress_equal_stacks=0 suppress_equal_addresses=0"

// TestRegisterGuard_firstLedgerOpenAgainstConcurrentOpens: the first ledger
// open of a process, through each door, while eight goroutines keep opening
// conversation stores on the same driver. The race detector must report
// nothing in any child, and each child must observe every opener's witness
// before the ledger open began, or the dangerous interleaving was never
// forced and the row fails. The read-only door registers nothing; its row is
// the same concurrency against the door that installs no guard, and it must
// refuse the empty file with ErrNoActionStore.
//
// The probe row opens no ledger. Its child reads, at the instant a ledger
// open would begin, a value each opener wrote right after its first open: if
// the wait ordered an opener before the ledger's goroutine, the read of its
// value would be ordered too and the detector would not report it. The row
// demands one report on readProbe per opener.
func TestRegisterGuard_firstLedgerOpenAgainstConcurrentOpens(t *testing.T) {
	if !raceEnabled {
		t.Skip("the mould's oracle is the race detector: run it with -race (make quality does)")
	}
	t.Run("probe", func(t *testing.T) {
		text, err := runRaceChild("probe", raceProbeGORACE)
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("probe: the child must fail on the race it was shown, got %v:\n%s", err, text)
		}
		reports, onProbe := strings.Count(text, "WARNING: DATA RACE"), strings.Count(text, ".readProbe(")
		if onProbe < raceOpeners {
			t.Fatalf("probe: %d race reports on readProbe, want one per opener (%d) — the mould's wait orders an opener before the ledger's goroutine:\n%s", onProbe, raceOpeners, text)
		}
		if reports != raceOpeners || onProbe != raceOpeners {
			t.Fatalf("probe: %d race reports, %d on readProbe, want exactly %d on readProbe — a race the probe did not plant:\n%s", reports, onProbe, raceOpeners, text)
		}
		if ahead := openedAhead(text); ahead != raceOpeners {
			t.Fatalf("probe: %d of %d openers had returned from an open before the probe was read:\n%s", ahead, raceOpeners, text)
		}
	})
	for _, door := range []string{"for", "operator", "readonly"} {
		t.Run(door, func(t *testing.T) {
			for child := 1; child <= raceChildren; child++ {
				text, err := runRaceChild(door)
				if strings.Contains(text, "DATA RACE") {
					t.Fatalf("door %s, child %d: the race detector fired:\n%s", door, child, text)
				}
				if err != nil {
					t.Fatalf("door %s, child %d: the child failed: %v\n%s", door, child, err, text)
				}
				if ahead := openedAhead(text); ahead != raceOpeners {
					t.Fatalf("door %s, child %d: %d of %d openers had returned from an open before the ledger open began — the interleaving was not forced:\n%s", door, child, ahead, raceOpeners, text)
				}
			}
		})
	}
}

// runRaceChild re-executes this test binary as the child for door, with env
// appended to its environment.
func runRaceChild(door string, env ...string) (string, error) {
	// #nosec G204 -- the mould deliberately re-executes this test binary.
	cmd := exec.Command(os.Args[0], "-test.run=^TestRegisterGuard_child$", "-test.count=1", "-test.v")
	cmd.Env = append(append(os.Environ(), raceChildEnv+"="+door), env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// openedAhead parses the child's report; -1 when it never reported.
func openedAhead(text string) int {
	ahead := -1
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(l, "RACECHILD ") {
			continue
		}
		for _, f := range strings.Fields(l) {
			if v, ok := strings.CutPrefix(f, "opened_ahead="); ok {
				ahead, _ = strconv.Atoi(v)
			}
		}
	}
	return ahead
}

// TestRegisterGuard_child is not a mould: it is the child process of
// TestRegisterGuard_firstLedgerOpenAgainstConcurrentOpens.
func TestRegisterGuard_child(t *testing.T) {
	door := os.Getenv(raceChildEnv)
	if door == "" {
		t.Skip("the register-guard race mould's child; run only by its mould")
	}
	dir := t.TempDir()
	ledger := filepath.Join(dir, "ledger.db")
	if door == "readonly" {
		// The read-only door opens an existing file only. It is laid down
		// without the driver and before any opener starts, so no ledger open
		// has happened in this process before the row's; an empty file is not
		// a korvun store and the door refuses it after opening its connection.
		if err := os.WriteFile(ledger, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	opens := make([]int, raceOpeners)
	probe := make([]int, raceOpeners)
	errs := make([]error, raceOpeners)
	var wg sync.WaitGroup
	park := make(chan struct{})
	for i := 0; i < raceOpeners; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := filepath.Join(dir, "conv-"+strconv.Itoa(i)+".db")
			for time.Since(start) < raceDeadline && !ledgerDone(dir) {
				s, err := convsqlite.Open(path)
				if err != nil {
					errs[i] = err
					return
				}
				opens[i]++
				if opens[i] == 1 {
					probe[i] = 1
					if err := os.Mkdir(witnessPath(dir, i), 0o700); err != nil {
						errs[i] = err
						_ = s.Close()
						return
					}
				}
				_ = s.Close()
				if door == "probe" {
					<-park
					return
				}
			}
		}(i)
	}

	waitForWitnesses(dir, start)
	ahead := countWitnesses(dir)
	var openErr error
	switch door {
	case "probe":
		readProbe(probe)
		close(park)
	case "for":
		var s *Store
		s, openErr = OpenFor(ledger, profileA)
		if s != nil {
			defer func() { _ = s.Close() }()
		}
	case "operator":
		var s *Store
		s, openErr = OpenOperatorFor(ledger, profileA)
		if s != nil {
			defer func() { _ = s.Close() }()
		}
	case "readonly":
		var s *Store
		s, openErr = OpenReadOnlyFor(ledger, profileA)
		if s != nil {
			defer func() { _ = s.Close() }()
		}
	default:
		t.Fatalf("unknown door %q", door)
	}
	if err := os.Mkdir(filepath.Join(dir, "ledger-done"), 0o700); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	switch {
	case door == "readonly":
		if !errors.Is(openErr, ErrNoActionStore) {
			t.Fatalf("door readonly: open the empty file: got %v, want ErrNoActionStore", openErr)
		}
	case openErr != nil:
		t.Fatalf("door %s: open the ledger: %v", door, openErr)
	}
	total := 0
	for i := 0; i < raceOpeners; i++ {
		if errs[i] != nil {
			t.Fatalf("opener %d: %v", i, errs[i])
		}
		total += opens[i]
	}
	fmt.Printf("RACECHILD door=%s opened_ahead=%d opens_total=%d ledger_open_err=%v\n", door, ahead, total, openErr)
}

// readProbe reads every opener's probe; the probe row's race report names it.
//
//go:noinline
func readProbe(probe []int) int {
	n := 0
	for _, v := range probe {
		n += v
	}
	return n
}

// witnessPath is the directory opener i creates once its first open returned.
func witnessPath(dir string, i int) string {
	return filepath.Join(dir, "opened-"+strconv.Itoa(i))
}

// ledgerDone reports whether the ledger's goroutine has announced that its
// open returned.
func ledgerDone(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "ledger-done"))
	return err == nil
}

// waitForWitnesses polls for every opener's witness until raceDeadline runs
// out. It decides nothing: countWitnesses is the oracle, and a wait that ends
// short leaves a count the parent refuses.
func waitForWitnesses(dir string, start time.Time) {
	for countWitnesses(dir) < raceOpeners && time.Since(start) < raceDeadline {
		time.Sleep(5 * time.Millisecond)
	}
}

// countWitnesses counts the openers whose witness is on disk.
func countWitnesses(dir string) int {
	n := 0
	for i := 0; i < raceOpeners; i++ {
		if _, err := os.Stat(witnessPath(dir, i)); err == nil {
			n++
		}
	}
	return n
}
