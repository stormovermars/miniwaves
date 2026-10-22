package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
)

// mw configure: walk the per-user settings (userkeys.go) with plain stdin/stdout prompts, no TUI library.
// Nothing is written before the Save answer. stdinIsTerminal (gates.go) and configureIn are the test seams.

var configureIn io.Reader = os.Stdin

const configureHint = "hint: run mw configure in a terminal to change these interactively, or mw configure KEY=VALUE... to set them"

func configureCommand(args []string) error {
	show, asJSON := false, false
	var sets []string
	for _, a := range args {
		switch {
		case a == "--show":
			show = true
		case a == "-j" || a == "--json":
			asJSON = true
		case strings.Contains(a, "=") && !strings.HasPrefix(a, "-"):
			sets = append(sets, a)
		default:
			return errors.New("usage: mw configure [--show [-j]] | mw configure KEY=VALUE...")
		}
	}
	if (show || asJSON) && len(sets) > 0 {
		return errors.New("usage: mw configure [--show [-j]] | mw configure KEY=VALUE...")
	}
	c, p, _, err := readUserConfig()
	if err != nil {
		return err
	}
	if show || asJSON {
		return printUserSettings(c, "", asJSON)
	}
	if len(sets) > 0 {
		for _, kv := range sets {
			k, v, _ := strings.Cut(kv, "=")
			if err := setUserKey(&c, strings.TrimSpace(k), v); err != nil {
				return err
			}
		}
		if err := writeUserConfig(c, p); err != nil {
			return err
		}
		fmt.Println("saved", p)
		return nil
	}
	if !stdinIsTerminal() {
		if err := printUserSettings(c, "", false); err != nil {
			return err
		}
		fmt.Println(configureHint)
		return nil
	}
	return configureInteractive(c, p, bufio.NewReader(configureIn), os.Stdout)
}

func cloneUserConfig(c UserConfig) UserConfig {
	n := c
	n.Settings = map[string]string{}
	for k, v := range c.Settings {
		n.Settings[k] = v
	}
	if c.Fanfare.Enabled != nil {
		b := *c.Fanfare.Enabled
		n.Fanfare.Enabled = &b
	}
	return n
}

func configureInteractive(orig UserConfig, p string, in *bufio.Reader, out io.Writer) error {
	nc := cloneUserConfig(orig)
	line := func() (string, bool) {
		s, err := in.ReadString('\n')
		if err != nil && s == "" {
			return "", false
		}
		return strings.TrimSpace(s), true
	}
	discard := func(why string) error {
		fmt.Fprintln(out, why)
		return nil
	}
	fmt.Fprintf(out, "mw configure: your personal settings (%s)\nEnter keeps the current value, a number picks a choice, d resets to the default.\n", p)
	for _, g := range userGroups {
		fmt.Fprintf(out, "\n== %s ==\n", g.Label)
		for i := range userKeys {
			k := &userKeys[i]
			if k.Group != g.ID {
				continue
			}
			for {
				cur, src := userValue(nc, k)
				fmt.Fprintf(out, "\n%s: %s\n  current: %s (%s)   allowed: %s\n", k.Name, k.Help, cur, src, k.allowed())
				numbered := k.Kind == "enum" || k.Kind == "bool"
				if numbered {
					for n, ch := range k.Choices {
						fmt.Fprintf(out, "  %d) %s\n", n+1, ch)
					}
				}
				extra := ""
				if g.ID == "fanfare" {
					extra = ", t to test-play"
				}
				fmt.Fprintf(out, "  [Enter keeps%s%s] > ", map[bool]string{true: ", number picks", false: ", type a value"}[numbered], extra)
				ans, ok := line()
				if !ok {
					return discard("input ended: nothing saved.")
				}
				if ans == "" {
					break
				}
				if g.ID == "fanfare" && strings.EqualFold(ans, "t") {
					testPlayFanfare(nc, out)
					continue
				}
				if strings.EqualFold(ans, "d") {
					_ = setUserKey(&nc, k.Name, "")
					break
				}
				val := ans
				if n, err := strconv.Atoi(ans); numbered && err == nil {
					if n < 1 || n > len(k.Choices) {
						fmt.Fprintf(out, "  choose 1 to %d\n", len(k.Choices))
						continue
					}
					val = k.Choices[n-1]
				}
				if err := setUserKey(&nc, k.Name, val); err != nil {
					fmt.Fprintln(out, "  "+err.Error())
					continue
				}
				break
			}
		}
	}
	fmt.Fprintln(out, "\n== Summary ==")
	changed := 0
	for i := range userKeys {
		k := &userKeys[i]
		ov, _ := userValue(orig, k)
		nv, ns := userValue(nc, k)
		if ov != nv || (ns == "user") != func() bool { _, s := userValue(orig, k); return s == "user" }() {
			changed++
			fmt.Fprintf(out, "  %s: %s -> %s\n", k.Name, ov, nv)
		}
	}
	if changed == 0 {
		fmt.Fprintln(out, "  no changes; nothing to save.")
		return nil
	}
	for {
		fmt.Fprint(out, "Save or Discard? [s/d] > ")
		ans, ok := line()
		if !ok {
			return discard("input ended: nothing saved.")
		}
		switch strings.ToLower(ans) {
		case "s", "save", "y", "yes":
			if err := writeUserConfig(nc, p); err != nil {
				return err
			}
			fmt.Fprintln(out, "saved", p)
			return nil
		case "d", "discard", "n", "no":
			return discard("discarded: nothing saved.")
		}
	}
}

// testPlayFanfare plays one milestone clip with the pending (unsaved) fanfare.use; it never fails configure.
func testPlayFanfare(c UserConfig, out io.Writer) {
	var w *Workspace
	if path, err := findWorkspace(); err == nil {
		if ww, err := load(path); err == nil {
			w = ww
		}
	}
	cfg := fanfareCfg(w)
	if c.Fanfare.Use != "" {
		cfg.Use = c.Fanfare.Use
	}
	dir := expandHome(cfg.Dir)
	pr, err := pickPairFor(cfg, dir, rand.Intn)
	if err == nil {
		err = playFile(dir, pr.Fanfare, cfg.MilestoneSeconds, cfg.Volume)
	}
	if err != nil {
		fmt.Fprintln(out, "  test play failed:", err)
		return
	}
	fmt.Fprintf(out, "  playing %s (%ds): %s\n", pr.Name, cfg.MilestoneSeconds, pr.Fanfare)
}
