package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"syscall"
	"time"
)

// mw log --follow [--exec]: a live feed of board events (oldest first as they arrive) and, with --exec, the
// per-user alerts.exec command run for each event whose action is listed in alerts.on.
//
// The command is an argv array and never goes through a shell. The event is written as JSON to a private temp
// file whose path is appended as the last argument; stdin is closed (the null device). MW_ALERT_EXEC (the same
// JSON array) replaces alerts.exec: it is the stub the tests use. An empty alerts.on selects nothing.

const alertExecTimeout = 30 * time.Second

// alertExecWaitDelay bounds how long Run waits for the child's pipes after it is killed or exits: a grandchild that
// keeps stderr open must not hang the feed.
var alertExecWaitDelay = 5 * time.Second

// The first read of a "from now" feed is retried while the board is mid-write.
const followStartAttempts = 5

var followStartBackoff = 200 * time.Millisecond

// fatalFollowError marks an error that retrying cannot fix (bad options).
type fatalFollowError struct{ err error }

func (e *fatalFollowError) Error() string { return e.err.Error() }

// alertArgv is the command to run: MW_ALERT_EXEC when set, else alerts.exec of the user config.
func alertArgv() ([]string, error) {
	if v := os.Getenv("MW_ALERT_EXEC"); v != "" {
		return parseAlertExec(v)
	}
	c, _, _, err := readUserConfig()
	if err != nil {
		return nil, err
	}
	if len(c.Alerts.Exec) == 0 {
		return nil, errors.New(`--exec needs alerts.exec: mw user set alerts.exec='["notify-send","mw"]' alerts.on=block,ask`)
	}
	return c.Alerts.Exec, nil
}

// alertOn is the set of event actions that run the command.
func alertOn() (map[string]bool, error) {
	c, _, _, err := readUserConfig()
	if err != nil {
		return nil, err
	}
	on := map[string]bool{}
	for _, n := range c.Alerts.On {
		on[n] = true
	}
	return on, nil
}

// runAlertExec writes the event to a temp file and runs argv + [file]; it never fails the feed.
func runAlertExec(argv []string, r logRow, errw io.Writer) {
	f, err := os.CreateTemp("", "mw-alert-*.json") // mode 0600
	if err != nil {
		fmt.Fprintf(errw, "mw: alerts.exec: %v\n", err)
		return
	}
	defer os.Remove(f.Name())
	b, _ := json.Marshal(r)
	_, werr := f.Write(append(b, '\n'))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		fmt.Fprintf(errw, "mw: alerts.exec: %v\n", werr)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), alertExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], append(append([]string{}, argv[1:]...), f.Name())...)
	cmd.Stdin = nil // the null device
	cmd.Stderr = errw
	cmd.WaitDelay = alertExecWaitDelay
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(errw, "mw: alerts.exec %s (%s %s): %v\n", argv[0], r.Item, r.Action, err)
	}
}

func logFollowCommand(o map[string]string) error {
	poll := 2 * time.Second
	if o["poll"] != "" {
		n, err := strconv.ParseFloat(o["poll"], 64)
		if err != nil || n < 0.05 || n > 3600 {
			return errors.New("--poll needs seconds between 0.05 and 3600")
		}
		poll = time.Duration(n * float64(time.Second))
	}
	stop := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		<-sig
		close(stop)
	}()
	return logFollow(o, os.Stdout, os.Stderr, poll, stop)
}

// logFollow polls the board (a read, no lock) every poll until stop closes. It starts from now unless --since
// names a cursor ("start" replays everything).
func logFollow(o map[string]string, out, errw io.Writer, poll time.Duration, stop <-chan struct{}) error {
	loop, err := startFollow(o, out, errw, poll, stop)
	if err != nil || loop == nil {
		return err
	}
	return loop()
}

// startFollow does everything that reads run state (board location, tenant selection, the first cursors) and returns
// the polling loop, which touches none of it: a caller may run the loop on its own goroutine while later commands
// reset that state. A nil loop with a nil error means stop closed during start.
func startFollow(o map[string]string, out, errw io.Writer, poll time.Duration, stop <-chan struct{}) (func() error, error) {
	if o["limit"] != "" || o["since-file"] != "" {
		return nil, errors.New("--follow cannot be combined with -n or --since-file")
	}
	var argv []string
	var on map[string]bool
	if o["exec"] == "true" {
		var err error
		if argv, err = alertArgv(); err != nil {
			return nil, err
		}
		if on, err = alertOn(); err != nil {
			return nil, err
		}
		if len(on) == 0 {
			fmt.Fprintln(errw, "mw: alerts.on is empty: --exec runs for no event (mw user set alerts.on=block,ask)")
		}
	}
	path, err := findWorkspace()
	if err != nil {
		return nil, err
	}
	// The tenant list is fixed at start (-A included): a tenant added later is not followed. Each tenant keeps its
	// own cursor, so a tenant that disappears (archive) and reappears (restore) does not replay its history.
	var slugs []string
	curs := map[string]string{}
	pick := func(w *Workspace) error {
		if slugs != nil {
			return nil
		}
		if o["all"] == "true" {
			if o["tenant"] != "" {
				return errors.New("mw log -A cannot be combined with -t")
			}
			for _, t := range shown(w, o) {
				slugs = append(slugs, t.Slug)
			}
		} else {
			t, err := selectTenantRaw(w, o)
			if err != nil {
				return err
			}
			slugs = []string{t.Slug}
		}
		for _, s := range slugs {
			curs[s] = o["since"]
		}
		return nil
	}
	_, hasSince := o["since"]
	step := func() ([]logRow, error) {
		w, err := load(path)
		if err != nil {
			return nil, err
		}
		if err := pick(w); err != nil {
			return nil, &fatalFollowError{err}
		}
		var all []logRow
		next := map[string]string{}
		for _, s := range slugs {
			t, err := tenantBySlug(w, s)
			if err != nil {
				continue // gone for now: keep its cursor for when it comes back
			}
			rows, c, err := logSince([]*Tenant{t}, curs[s], 0)
			if err != nil {
				return nil, err
			}
			all = append(all, rows...)
			next[s] = c
		}
		for s, c := range next { // commit only after every tenant read cleanly
			curs[s] = c
		}
		sort.SliceStable(all, func(i, j int) bool {
			x, y := parseTS(all[i].At), parseTS(all[j].At)
			if !x.Equal(y) {
				return x.Before(y)
			}
			if all[i].Tenant != all[j].Tenant {
				return all[i].Tenant < all[j].Tenant
			}
			return all[i].Item < all[j].Item
		})
		return all, nil
	}
	// Start: from now takes the cursors; with --since only the tenant is resolved (here, not on the loop's
	// goroutine, which must read no run state). A board mid-write is retried a few times; a pick error is fatal.
	first := func() error {
		if !hasSince {
			_, err := step()
			return err
		}
		w, err := load(path)
		if err != nil {
			return err
		}
		if err := pick(w); err != nil {
			return &fatalFollowError{err}
		}
		return nil
	}
	{
		var err error
		for i := 0; i < followStartAttempts; i++ {
			if err = first(); err == nil {
				break
			}
			var fe *fatalFollowError
			if errors.As(err, &fe) {
				return nil, fe.err
			}
			if i < followStartAttempts-1 {
				select {
				case <-stop:
					return nil, nil
				case <-time.After(time.Duration(i+1) * followStartBackoff):
				}
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return func() error {
		for {
			rows, err := step()
			if err != nil {
				fmt.Fprintf(errw, "mw: log --follow: %v\n", err) // a board mid-write or mid-merge: try again
			}
			for _, r := range rows {
				if o["json"] == "true" {
					b, _ := json.Marshal(r)
					fmt.Fprintln(out, string(b))
				} else {
					by := r.By
					if by == "" {
						by = "-"
					}
					line := fmt.Sprintf("%s  %s  %-8s %-9s %s", shortTS(r.At), r.Tenant, r.Item, r.Action, by)
					if r.Note != "" {
						line += "  " + compactText(r.Note, 90)
					}
					fmt.Fprintln(out, line)
				}
				if argv != nil && on[r.Action] {
					runAlertExec(argv, r, errw)
				}
			}
			select {
			case <-stop:
				return nil
			case <-time.After(poll):
			}
		}
	}, nil
}
