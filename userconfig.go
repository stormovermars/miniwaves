package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Per-user config (mw-user.json): personal choices that never ride along with the committed board.
// It lives in os.UserConfigDir()/miniwaves/ (MW_USER_CONFIG overrides the path, for tests and odd setups).
// On Unix a file that is group- or world-writable is refused, since alerts.exec names a command.
// Windows has no comparable mode bits, so no check is made there.

const userConfigName = "mw-user.json"

var (
	alertNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

// userModeCheck is false on Windows.
var userModeCheck = runtime.GOOS != "windows"

type UserConfig struct {
	Alerts  UserAlerts  `json:"alerts"`
	Fanfare UserFanfare `json:"fanfare"`
	// Settings holds the table keys of userkeys.go (pane.*, alerts.kinds, status.*, ...) as canonical strings.
	Settings map[string]string `json:"settings,omitempty"`
}

type UserAlerts struct {
	On   []string `json:"on,omitempty"`   // event names
	Exec []string `json:"exec,omitempty"` // an argv array; never run through a shell
}

type UserFanfare struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Use     string `json:"use,omitempty"` // a set name or "all"
}

func userConfigPath() (string, error) {
	if p := os.Getenv("MW_USER_CONFIG"); p != "" {
		return p, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate the per-user config folder (%v): set MW_USER_CONFIG", err)
	}
	return filepath.Join(d, "miniwaves", userConfigName), nil
}

// readUserConfig returns the config and whether the file exists. A missing file is an empty config.
func readUserConfig() (UserConfig, string, bool, error) {
	var c UserConfig
	p, err := userConfigPath()
	if err != nil {
		return c, "", false, err
	}
	fi, err := os.Stat(p)
	if os.IsNotExist(err) {
		return c, p, false, nil
	}
	if err != nil {
		return c, p, false, err
	}
	if userModeCheck && fi.Mode().Perm()&0o022 != 0 {
		return c, p, true, fmt.Errorf("%s is writable by group or others (mode %04o): run chmod 600 %s; mw will not read it, because alerts.exec names a command", p, fi.Mode().Perm(), p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return c, p, true, err
	}
	if len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &c); err != nil {
			return UserConfig{}, p, true, fmt.Errorf("read %s: %w", p, err)
		}
	}
	return c, p, true, nil
}

func setUserKey(c *UserConfig, key, val string) error {
	if !noControl(val) {
		return fmt.Errorf("%s: no control characters", key)
	}
	switch key {
	case "alerts.on":
		var names []string
		seen := map[string]bool{}
		for _, n := range strings.Split(val, ",") {
			if n = strings.ToLower(strings.TrimSpace(n)); n == "" {
				continue
			}
			if !alertNamePattern.MatchString(n) {
				return fmt.Errorf("alerts.on: %q is not an event name (lowercase letters, digits, . _ -)", n)
			}
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
		c.Alerts.On = names
	case "alerts.exec":
		if strings.TrimSpace(val) == "" {
			c.Alerts.Exec = nil
			return nil
		}
		var argv []string
		if err := json.Unmarshal([]byte(val), &argv); err != nil {
			return errors.New(`alerts.exec: must be a JSON array of strings, for example ["notify-send","mw"]`)
		}
		if len(argv) == 0 || len(argv) > 32 {
			return errors.New("alerts.exec: 1 to 32 arguments")
		}
		if argv[0] == "" {
			return errors.New("alerts.exec: the program (first element) must not be empty")
		}
		for _, a := range argv {
			if len(a) > 4096 || strings.ContainsRune(a, 0) {
				return errors.New("alerts.exec: arguments are at most 4096 bytes, with no NUL")
			}
		}
		c.Alerts.Exec = argv
	case "fanfare.enabled":
		if val == "" {
			c.Fanfare.Enabled = nil
			return nil
		}
		b, err := strconv.ParseBool(val)
		if err != nil || val != "true" && val != "false" {
			return fmt.Errorf("fanfare.enabled: %q must be true or false", val)
		}
		c.Fanfare.Enabled = &b
	case "fanfare.use":
		if val != "" && val != "all" && !setNameRe.MatchString(val) {
			return fmt.Errorf("fanfare.use: %q must be a set name or all", val)
		}
		c.Fanfare.Use = val
	default:
		k := userKeyByName(key)
		if k == nil {
			return fmt.Errorf("unknown user key %q (keys: alerts.on, alerts.exec, %s)", key, strings.Join(userKeyNames(), ", "))
		}
		if val == "" {
			delete(c.Settings, key)
			return nil
		}
		v, err := k.normalize(val)
		if err != nil {
			return err
		}
		if c.Settings == nil {
			c.Settings = map[string]string{}
		}
		c.Settings[key] = v
	}
	return nil
}

func writeUserConfig(c UserConfig, p string) error {
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(p, append(b, '\n'))
}

// userView is the -j form of mw user show.
type userView struct {
	Path           string   `json:"path"`
	Exists         bool     `json:"exists"`
	AlertsOn       []string `json:"alerts_on"`
	AlertsExec     []string `json:"alerts_exec"`
	FanfareEnabled *bool    `json:"fanfare_enabled"`
	FanfareUse     string   `json:"fanfare_use"`
}

func userCommand(args []string) error {
	const use = "usage: mw user show [-j] | mw user get [KEY] [-j] | mw user set KEY=VALUE...  (keys: alerts.on, alerts.exec, " + "fanfare.enabled, fanfare.use, ...: mw user get lists them)"
	if len(args) == 0 {
		return errors.New(use)
	}
	o, pos, err := options(args[1:])
	if err != nil {
		return err
	}
	if err := allowOptions(o, "json"); err != nil {
		return err
	}
	c, p, exists, err := readUserConfig()
	if err != nil {
		return err
	}
	switch args[0] {
	case "show":
		if len(pos) != 0 {
			return errors.New(use)
		}
		v := userView{Path: p, Exists: exists, AlertsOn: c.Alerts.On, AlertsExec: c.Alerts.Exec, FanfareEnabled: c.Fanfare.Enabled, FanfareUse: c.Fanfare.Use}
		if v.AlertsOn == nil {
			v.AlertsOn = []string{}
		}
		if v.AlertsExec == nil {
			v.AlertsExec = []string{}
		}
		if o["json"] == "true" {
			return printJSON(v)
		}
		unset := func(s string) string {
			if s == "" {
				return "(unset)"
			}
			return s
		}
		en := ""
		if c.Fanfare.Enabled != nil {
			en = strconv.FormatBool(*c.Fanfare.Enabled)
		}
		ex := ""
		if len(c.Alerts.Exec) > 0 {
			b, _ := json.Marshal(c.Alerts.Exec)
			ex = string(b)
		}
		note := ""
		if !exists {
			note = " (not created yet)"
		}
		fmt.Printf("path: %s%s\nalerts.on: %s\nalerts.exec: %s\nfanfare.enabled: %s\nfanfare.use: %s\n",
			p, note, unset(strings.Join(c.Alerts.On, ",")), unset(ex), unset(en), unset(c.Fanfare.Use))
		return nil
	case "set":
		if len(pos) == 0 {
			return errors.New("usage: mw user set KEY=VALUE...")
		}
		for _, kv := range pos {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("%q is not KEY=VALUE", kv)
			}
			if err := setUserKey(&c, strings.TrimSpace(k), v); err != nil {
				return err
			}
		}
		if err := writeUserConfig(c, p); err != nil {
			return err
		}
		fmt.Println("saved", p)
		return nil
	case "get":
		if len(pos) > 1 {
			return errors.New(use)
		}
		key := ""
		if len(pos) == 1 {
			key = strings.TrimSpace(pos[0])
			if userKeyByName(key) == nil {
				return fmt.Errorf("unknown user key %q (keys: %s)", key, strings.Join(userKeyNames(), ", "))
			}
		}
		return printUserSettings(c, key, o["json"] == "true")
	}
	return errors.New(use)
}

var userConfigWarned bool

// fanfareEffective is the board's fanfare config with the per-user fanfare.enabled applied on top
// (the user's choice wins over the committed board). A broken user file is ignored with one stderr note.
func fanfareEffective(w *Workspace) FanfareConfig {
	c, _ := fanfareEffectiveSrc(w)
	return c
}

// fanfareEffectiveSrc is fanfareEffective plus where enabled and use came from ("user" or "board").
func fanfareEffectiveSrc(w *Workspace) (FanfareConfig, map[string]string) {
	c := fanfareCfg(w)
	src := map[string]string{"enabled": "board", "use": "board"}
	u, _, _, err := readUserConfig()
	if err != nil {
		if !userConfigWarned {
			userConfigWarned = true
			fmt.Fprintln(os.Stderr, "mw: per-user config ignored:", err)
		}
		return c, src
	}
	if u.Fanfare.Enabled != nil {
		c.Enabled, src["enabled"] = *u.Fanfare.Enabled, "user"
	}
	if u.Fanfare.Use != "" {
		c.Use, src["use"] = u.Fanfare.Use, "user"
	}
	return c, src
}
