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
	if o["limit"] != "" || o["since-file"] != "" {
		return errors.New("--follow cannot be combined with -n or --since-file")
	}
	var argv []string
	var on map[string]bool
	if o["exec"] == "true" {
		var err error
		if argv, err = alertArgv(); err != nil {
			return err
		}
		if on, err = alertOn(); err != nil {
			return err
		}
		if len(on) == 0 {
			fmt.Fprintln(errw, "mw: alerts.on is empty: --exec runs for no event (mw user set alerts.on=block,ask)")
		}
	}
	path, err := findWorkspace()
	if err != nil {
		return err
	}
	var slugs []string
	pick := func(w *Workspace) ([]*Tenant, error) {
		if slugs == nil {
			if o["all"] == "true" {
				if o["tenant"] != "" {
					return nil, errors.New("mw log -A cannot be combined with -t")
				}
				for _, t := range shown(w, o) {
					slugs = append(slugs, t.Slug)
				}
			} else {
				t, err := selectTenantRaw(w, o)
				if err != nil {
					return nil, err
				}
				slugs = []string{t.Slug}
			}
		}
		var ts []*Tenant
		for _, s := range slugs {
			if t, err := tenantBySlug(w, s); err == nil {
				ts = append(ts, t)
			}
		}
		return ts, nil
	}
	cursor := o["since"]
	_, hasSince := o["since"]
	step := func() ([]logRow, error) {
		w, err := load(path)
		if err != nil {
			return nil, err
		}
		ts, err := pick(w)
		if err != nil {
			return nil, err
		}
		rows, next, err := logSince(ts, cursor, 0)
		if err != nil {
			return nil, err
		}
		cursor = next
		return rows, nil
	}
	if !hasSince { // from now: take the cursor, deliver nothing
		if _, err := step(); err != nil {
			return err
		}
	}
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
}
