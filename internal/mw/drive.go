package mw

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// mw drive records a session-to-session drive on both boards. mw sends nothing: the messages travel
// over SendMessage; mw formats and parses the one-line wire format and keeps the two records in agreement.
// See USAGE.md "Drive" and TECHNICAL.md "Drive wire format".

const (
	driveTag      = "mw-drive"
	driveTextMax  = 400
	driveIDMax    = 30
	driveSeqMax   = 999999
	driveTicketPf = "drive."
)

// driveIn is where `drive recv` reads the line from when no argument is given (a test seam).
var driveIn io.Reader = os.Stdin

// nowFn is the clock of drive status and check. Only `drive check --at` overrides it.
var nowFn = func() time.Time { return time.Now().UTC() }

var driveIDRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*\.[a-z0-9.-]+$`)
var driveSlugRe = regexp.MustCompile(`^[a-z0-9.-]+$`)
var driveLabelRe = regexp.MustCompile(`\[data from ([a-z0-9-]+)\]`)

// ---- wire format (pure) ----

type driveMsg struct {
	ID      string
	Seq     int
	Event   string
	N       int
	To      string // open: the remote member the line is addressed to
	Verdict string // result: pass|fail
	Text    string // open goal, task/steer/note/result text, pause "idle", resumed session, busy member
}

var driverEvents = map[string]bool{"open": true, "task": true, "steer": true, "stop": true, "close": true, "note": true, "resumed": true}
var remoteEvents = map[string]bool{"busy": true, "result": true, "note": true, "pause": true, "closed": true, "resumed": true}

func driveUsage(format string, a ...any) error {
	return coded("USAGE", fmt.Errorf(format, a...))
}

// driveState is an event that does not fit the drive's state (exit 2, code DRIVE_STATE).
func driveState(format string, a ...any) error {
	return &exitError{code: 2, msg: fmt.Sprintf(format, a...), ccode: "DRIVE_STATE"}
}

func checkDriveText(text string, required bool) error {
	if text == "" {
		if required {
			return driveUsage("drive: text is required")
		}
		return nil
	}
	if len(text) > driveTextMax {
		return driveUsage("drive: text is over %d characters", driveTextMax)
	}
	for i := 0; i < len(text); i++ {
		if c := text[i]; c < 0x20 || c > 0x7e {
			return driveUsage("drive: text must be printable ASCII on one line (no newline, control or non-ASCII characters)")
		}
		if c := text[i]; c == '[' || c == ']' {
			return driveUsage("drive: text must not contain [ or ] (they delimit the [data from MEMBER] label)")
		}
	}
	if text != strings.TrimSpace(text) {
		return driveUsage("drive: text must not start or end with a space")
	}
	return nil
}

func checkDriveID(id string) error {
	if len(id) > driveIDMax {
		return driveUsage("drive: id %q is over %d characters", id, driveIDMax)
	}
	if !driveIDRe.MatchString(id) {
		return driveUsage("drive: id %q must be MEMBER.SLUG from [a-z0-9.-]", id)
	}
	return nil
}

// validate checks a message against the event grammar.
func (m driveMsg) validate() error {
	if err := checkDriveID(m.ID); err != nil {
		return err
	}
	if m.Seq < 1 || m.Seq > driveSeqMax {
		return driveUsage("drive: seq must be 1 to %d", driveSeqMax)
	}
	needN, textReq, textOK := false, false, false
	switch m.Event {
	case "open":
		if !slugPattern.MatchString(m.To) {
			return driveUsage("drive: open needs the remote member name")
		}
		textReq, textOK = true, true
	case "task", "steer":
		needN, textReq, textOK = true, true, true
	case "note":
		needN, textReq, textOK = true, true, true
	case "result":
		needN, textOK = true, true
		if m.Verdict != "pass" && m.Verdict != "fail" {
			return driveUsage("drive: result needs pass or fail")
		}
	case "stop", "close", "closed":
	case "pause":
		if m.Text != "idle" {
			return driveUsage("drive: pause takes the word idle")
		}
		return nil
	case "resumed":
		if !validID(m.Text) || m.Text == localName {
			return driveUsage("drive: resumed needs a session name")
		}
		return nil
	case "busy":
		if !slugPattern.MatchString(m.Text) {
			return driveUsage("drive: busy needs a member name")
		}
		return nil
	default:
		return driveUsage("drive: unknown event %q", m.Event)
	}
	if needN {
		min := 1
		if m.Event == "note" {
			min = 0
		}
		if m.N < min || m.N > driveSeqMax {
			return driveUsage("drive: %s needs a task number (%d to %d)", m.Event, min, driveSeqMax)
		}
	} else if m.N != 0 {
		return driveUsage("drive: %s takes no task number", m.Event)
	}
	if m.Verdict != "" && m.Event != "result" {
		return driveUsage("drive: %s takes no verdict", m.Event)
	}
	if !textOK && m.Text != "" {
		return driveUsage("drive: %s takes no text", m.Event)
	}
	return checkDriveText(m.Text, textReq)
}

// head is the event with its numbers, as in the wire line and the note.
func (m driveMsg) head() string {
	h := m.Event
	switch m.Event {
	case "open":
		h += " " + m.To
	case "task", "steer", "note":
		h += " " + strconv.Itoa(m.N)
	case "result":
		h += " " + strconv.Itoa(m.N) + " " + m.Verdict
	case "pause", "resumed", "busy":
		h += " " + m.Text
	}
	return h
}

// body is the free text of the event ("" when the event has none).
func (m driveMsg) body() string {
	switch m.Event {
	case "open", "task", "steer", "note", "result":
		return m.Text
	}
	return ""
}

// formatDriveLine renders the wire line `mw-drive <id> <seq> <event> [n] [text...]`.
func formatDriveLine(m driveMsg) (string, error) {
	if err := m.validate(); err != nil {
		return "", err
	}
	s := fmt.Sprintf("%s %s %d %s", driveTag, m.ID, m.Seq, m.head())
	if b := m.body(); b != "" {
		s += " " + b
	}
	return s, nil
}

// parseDriveLine is the inverse of formatDriveLine; a malformed line is a USAGE error.
func parseDriveLine(line string) (driveMsg, error) {
	line = strings.TrimSpace(line)
	var m driveMsg
	bad := func(why string) (driveMsg, error) {
		return m, driveUsage("malformed drive line: %s (want: %s ID SEQ EVENT [N] [TEXT])", why, driveTag)
	}
	if line == "" {
		return bad("empty")
	}
	f := strings.SplitN(line, " ", 5)
	if f[0] != driveTag {
		return bad("does not start with " + driveTag)
	}
	if len(f) < 4 {
		return bad("too few fields")
	}
	m.ID = f[1]
	seq, err := strconv.Atoi(f[2])
	if err != nil || strconv.Itoa(seq) != f[2] {
		return bad("seq is not a number")
	}
	m.Seq, m.Event = seq, f[3]
	rest := ""
	if len(f) == 5 {
		rest = f[4]
	}
	if strings.ContainsAny(line, "\r\n") {
		return bad("newline in line")
	}
	switch m.Event {
	case "open":
		to, goal, _ := strings.Cut(rest, " ")
		m.To, m.Text = to, goal
	case "task", "steer", "note", "result":
		// rest = N [verdict] [TEXT]
		parts := strings.SplitN(rest, " ", 2)
		n, err := strconv.Atoi(parts[0])
		if parts[0] == "" || err != nil || strconv.Itoa(n) != parts[0] {
			return bad(m.Event + " needs a task number")
		}
		m.N = n
		tail := ""
		if len(parts) == 2 {
			tail = parts[1]
		}
		if m.Event == "result" {
			vt := strings.SplitN(tail, " ", 2)
			m.Verdict = vt[0]
			if len(vt) == 2 {
				m.Text = vt[1]
			}
		} else {
			m.Text = tail
		}
	case "pause", "resumed", "busy":
		m.Text = rest
	default: // stop, close, closed and unknown events: no arguments
		if rest != "" {
			return bad(m.Event + " takes no arguments")
		}
	}
	if err := m.validate(); err != nil {
		return m, driveUsage("malformed drive line: %s", strings.TrimPrefix(err.Error(), "drive: "))
	}
	return m, nil
}

// ---- board state ----

// driveInfo is the state kept in the story body: driver=<m> remote=<m> driver_session=<s> remote_session=<s> seq_in=<k> seq_out=<k>.
type driveInfo struct {
	Driver, Remote               string
	DriverSession, RemoteSession string
	SeqIn, SeqOut                int
}

func (d driveInfo) String() string {
	return fmt.Sprintf("driver=%s remote=%s driver_session=%s remote_session=%s seq_in=%d seq_out=%d",
		d.Driver, d.Remote, dash(d.DriverSession), dash(d.RemoteSession), d.SeqIn, d.SeqOut)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func undash(s string) string {
	if s == "-" {
		return ""
	}
	return s
}

func parseDriveInfo(body string) (driveInfo, bool) {
	var d driveInfo
	seen := 0
	for _, f := range strings.Fields(body) {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		switch k {
		case "driver":
			d.Driver = v
		case "remote":
			d.Remote = v
		case "driver_session":
			d.DriverSession = undash(v)
		case "remote_session":
			d.RemoteSession = undash(v)
		case "seq_in":
			d.SeqIn, _ = strconv.Atoi(v)
		case "seq_out":
			d.SeqOut, _ = strconv.Atoi(v)
		default:
			continue
		}
		seen++
	}
	return d, seen == 6 && d.Driver != "" && d.Remote != ""
}

// drive is one story of the tenant that records a drive.
type drive struct {
	t     *Tenant
	story *Item
	id    string
	info  driveInfo
}

func (d *drive) driverSide() bool { return d.t.Slug == d.info.Driver }
func (d *drive) open() bool       { return d.story.Status != "done" }
func (d *drive) peer() string {
	if d.driverSide() {
		return d.info.Remote
	}
	return d.info.Driver
}

func (d *drive) save() { d.story.Body = d.info.String() }

func findDrive(t *Tenant, id string) *drive {
	for i := range t.Items {
		it := &t.Items[i]
		if it.Kind == "story" && it.Ticket == driveTicketPf+id {
			if info, ok := parseDriveInfo(it.Body); ok {
				return &drive{t: t, story: it, id: id, info: info}
			}
		}
	}
	return nil
}

// drivesOf lists the drives of a tenant, in item order.
func drivesOf(t *Tenant) []*drive {
	var out []*drive
	for i := range t.Items {
		it := &t.Items[i]
		if it.Kind == "story" && strings.HasPrefix(it.Ticket, driveTicketPf) {
			if info, ok := parseDriveInfo(it.Body); ok {
				out = append(out, &drive{t: t, story: it, id: strings.TrimPrefix(it.Ticket, driveTicketPf), info: info})
			}
		}
	}
	return out
}

// remoteHeld returns the open drive the session holds as a remote, or nil.
func remoteHeld(t *Tenant, session string) *drive {
	for _, d := range drivesOf(t) {
		if d.open() && !d.driverSide() && d.story.Assignee == session && d.story.Status == "doing" {
			return d
		}
	}
	return nil
}

// driverHeld returns the open drive the session holds as a driver, or nil.
func driverHeld(t *Tenant, session string) *drive {
	for _, d := range drivesOf(t) {
		if d.open() && d.driverSide() && d.story.Assignee == session && d.story.Status == "doing" {
			return d
		}
	}
	return nil
}

// isDriveStory reports whether the item is the story of a drive (its ticket starts with drive.).
func isDriveStory(it Item) bool {
	return it.Kind == "story" && strings.HasPrefix(it.Ticket, driveTicketPf)
}

// inDrive reports whether the item is a child of a drive story.
func inDrive(t *Tenant, it Item) bool {
	if it.Parent == "" {
		return false
	}
	p, err := itemByID(t, it.Parent)
	return err == nil && isDriveStory(*p)
}

func driveChild(d *drive, n int) *Item {
	title := "drive " + strconv.Itoa(n)
	for i := range d.t.Items {
		it := &d.t.Items[i]
		if it.Parent == d.story.ID && it.Title == title {
			return it
		}
	}
	return nil
}

func driveTracked(w *Workspace, t *Tenant) error {
	if cfg, ok := w.Jobs[t.Job]; ok && t.Job != "" && cfg.Tracker != "" {
		return driveUsage("drive: tenant %s has a tracker (job %s); drive tickets are plain text, so drive is refused here", t.Slug, t.Job)
	}
	return nil
}

func driveNote(it *Item, now, session, text string) {
	who := ""
	if session != "" {
		who = "[" + session + "] "
	}
	it.Notes = append(it.Notes, now+" "+who+text)
	emit(it, now, "note", session, text)
	it.Updated = now
}

// recordText is the note of an event: `mw-drive <seq> <event> [n] [data from <member>] TEXT`.
func recordText(m driveMsg, sender string) string {
	s := fmt.Sprintf("%s %d %s [data from %s]", driveTag, m.Seq, m.head(), sender)
	if b := m.body(); b != "" {
		s += " " + b
	}
	return s
}

func newDriveItem(t *Tenant, kind, title, parent, ticket, body, now string) *Item {
	prefix := kinds[kind]
	t.Next[prefix]++
	it := Item{ID: fmt.Sprintf("%s-%03d", prefix, t.Next[prefix]), Kind: kind, Title: title, Status: "ready",
		Parent: parent, Ticket: ticket, Body: body, Created: now, Updated: now}
	emit(&it, now, "created", "", "")
	t.Items = append(t.Items, it)
	return &t.Items[len(t.Items)-1]
}

func claimStory(w *Workspace, it *Item, session, now string) {
	it.Status, it.Assignee, it.Updated, it.StartedOn = "doing", session, now, nodeName()
	emit(it, now, "start", session, "")
	// the story claim leaves the session's own item free, so the session can still start the drive's tasks
}

// judgeOn reports whether the tenant's judge gate is on.
func judgeOn(w *Workspace, t *Tenant) bool { return byName(gateStates(w, t), "judge").Effective }

// closeDone marks an item done without evidence, the way `mw done --no-evidence REASON` does: the reason is
// recorded, and with the judge gate on the item waits for review (Judged stays empty).
func closeDone(it *Item, now, session, reason string, judge bool) {
	it.Status, it.Assignee, it.Updated = "done", "", now
	it.By, it.JudgedBy, it.Evidence, it.Judged = displayName(session), "", nil, ""
	if !judge {
		it.Judged = "ungated"
	}
	it.Notes = append(it.Notes, now+" no-evidence: "+reason)
	emit(it, now, "done", session, reason)
}

// finishOpenChildren closes the children that are not done through the normal no-evidence path, on both boards.
func finishOpenChildren(w *Workspace, d *drive, now, session string) {
	judge := judgeOn(w, d.t)
	for i := range d.t.Items {
		c := &d.t.Items[i]
		if c.Parent != d.story.ID || c.Status == "done" {
			continue
		}
		releaseOwner(w, c)
		driveNote(c, now, session, "left open at close")
		closeDone(c, now, session, "not done: left open at drive close", judge)
	}
}

// closeStory finishes the open children, then marks the story done as a no-evidence close with the reason.
func closeStory(w *Workspace, d *drive, now, session, reason string) {
	finishOpenChildren(w, d, now, session)
	closeDone(d.story, now, session, reason, judgeOn(w, d.t))
}

// unclaimChildren puts the doing children of the story back to ready.
func unclaimChildren(w *Workspace, d *drive, now, session string) {
	for i := range d.t.Items {
		c := &d.t.Items[i]
		if c.Parent != d.story.ID || c.Status != "doing" {
			continue
		}
		releaseOwner(w, c)
		c.Status, c.Updated, c.Note, c.StartedOn = "ready", now, "", ""
		emit(c, now, "ready", session, "unclaimed: drive paused or stopped")
		c.Notes = append(c.Notes, now+" unclaimed: drive paused or stopped")
	}
}

func finishChildPass(w *Workspace, d *drive, c *Item, now, session, from string) {
	closeDone(c, now, session, "drive result from "+from, judgeOn(w, d.t))
	if !judgeOn(w, d.t) {
		unblockDependents(d.t, c.ID, now, displayName(session))
	}
}

// ---- applying events ----

// applyEvent applies m to the drive's story and children. local is true for a send (the caller is the sender).
// It returns the reply line of a recv ("" when none).
func applyEvent(w *Workspace, d *drive, m driveMsg, sender, session, now string, local bool) error {
	rec := recordText(m, sender)
	story := d.story
	childFor := func(n int) (*Item, error) {
		c := driveChild(d, n)
		if c == nil {
			return nil, driveState("drive %s has no task %d", d.id, n)
		}
		return c, nil
	}
	switch m.Event {
	case "task":
		if driveChild(d, m.N) != nil {
			return driveState("drive %s already has task %d", d.id, m.N)
		}
		c := newDriveItem(d.t, "task", "drive "+strconv.Itoa(m.N), story.ID, "", "", now)
		d.story, _ = itemByID(d.t, story.ID) // the append may have moved the items
		c.Status = "ready"                   // on the driver's board a mirror: pickNext skips drive children
		emit(c, now, "ready", session, "")
		driveNote(c, now, session, rec)
	case "steer":
		c, err := childFor(m.N)
		if err != nil {
			return err
		}
		if c.Status == "done" {
			return driveState("drive %s task %d is done", d.id, m.N)
		}
		driveNote(c, now, session, rec)
	case "note":
		if m.N == 0 {
			driveNote(story, now, session, rec)
		} else {
			c, err := childFor(m.N)
			if err != nil {
				return err
			}
			driveNote(c, now, session, rec)
		}
	case "stop", "pause":
		driveNote(story, now, session, rec)
		unclaimChildren(w, d, now, session)
	case "close", "closed", "busy":
		driveNote(story, now, session, rec)
		if story.Status != "done" {
			reason := "drive closed by " + sender
			if m.Event == "busy" {
				reason = "drive refused: busy"
			}
			closeStory(w, d, now, session, reason)
		}
	case "result":
		c, err := childFor(m.N)
		if err != nil {
			return err
		}
		pass := m.Verdict == "pass"
		if local && pass {
			// the remote's own board decides: the task must already be done through mw done (gates apply there)
			if c.Status != "done" {
				return driveState("drive %s task %d is %s: finish it with mw done first, then send result pass", d.id, m.N, c.Status)
			}
		} else if c.Status == "done" {
			return driveState("drive %s task %d is already done", d.id, m.N)
		}
		driveNote(c, now, session, rec)
		if pass && !local { // the driver's mirror closes as no-evidence
			finishChildPass(w, d, c, now, session, sender)
		}
	case "resumed":
		driveNote(story, now, session, rec)
		if local {
			if d.driverSide() {
				d.info.DriverSession = m.Text
			} else {
				d.info.RemoteSession = m.Text
			}
		} else if d.driverSide() {
			d.info.RemoteSession = m.Text
		} else {
			d.info.DriverSession = m.Text
		}
	default:
		return driveUsage("drive: cannot apply event %q", m.Event)
	}
	return nil
}

// ---- command ----

// driveLead splits the leading option tokens from the rest; the rest is literal (TEXT may contain -j, -s, --to).
// Options therefore come before the first non-option token (the event, the id or the goal); -- ends them early.
func driveLead(args []string, allowed ...string) (map[string]string, []string, error) {
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		key := ""
		if strings.HasPrefix(a, "--") {
			key = a[2:]
		} else if len(a) == 2 {
			key = flagAliases[a[1:]]
		}
		if key == "json" {
			i++
		} else {
			i += 2
		}
	}
	if i > len(args) {
		i = len(args)
	}
	o, _, err := options(args[:i])
	if err != nil {
		return nil, nil, err
	}
	if err := allowOptions(o, append(allowed, "json")...); err != nil {
		return nil, nil, err
	}
	return o, args[i:], nil
}

// driveOpts parses the drive options (the generic parser, for the commands without TEXT).
func driveOpts(args []string, allowed ...string) (map[string]string, []string, error) {
	o, pos, err := options(args)
	if err != nil {
		return nil, nil, err
	}
	if err := allowOptions(o, append(allowed, "json")...); err != nil {
		return nil, nil, err
	}
	return o, pos, nil
}

func driveCommand(path string, w *Workspace, args []string) error {
	if len(args) == 0 {
		return driveUsage("usage: mw drive send|recv|resume|status|check (mw help drive)")
	}
	switch args[0] {
	case "send":
		return driveSend(path, w, args[1:])
	case "recv":
		return driveRecv(path, w, args[1:])
	case "resume":
		return driveResume(path, w, args[1:])
	case "status":
		return driveStatus(w, args[1:])
	case "check":
		return driveCheck(w, args[1:])
	}
	return driveUsage("unknown drive command %q (mw help drive)", args[0])
}

// driveSession resolves the named session and the tenant it works in.
func driveSession(w *Workspace, o map[string]string) (string, *Tenant, error) {
	session := value(o, "session", os.Getenv("MW_SESSION"))
	if session == "" {
		return "", nil, driveUsage("drive needs a named session: use -s NAME or MW_SESSION (for example mac-a)")
	}
	if session == localName || !validID(session) {
		return "", nil, driveUsage("drive: session name must be at most 64 lowercase letters, numbers, and single dashes")
	}
	sess, err := sessionByID(w, session)
	if err != nil {
		return "", nil, driveUsage("drive: session %s is not registered: run mw session start %s -t TENANT first", session, session)
	}
	if sess.State == "stopped" {
		return "", nil, driveUsage("drive: session %s is stopped: start a new session (mw session start NAME -t TENANT)", session)
	}
	t, err := selectTenant(w, o)
	if err != nil {
		return "", nil, err
	}
	if err := driveTracked(w, t); err != nil {
		return "", nil, err
	}
	return session, t, nil
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339) }

// holder checks that the session holds the drive's story.
func (d *drive) holder(w *Workspace, session string) error {
	a := d.story.Assignee
	switch {
	case a == session:
		return nil
	case a == "":
		return driveState("drive %s is not held by any session: run mw drive resume %s", d.id, d.id)
	}
	if s, err := sessionByID(w, a); err != nil || s.State == "stopped" {
		return driveState("drive %s is held by dead session %s (stopped or unregistered): run mw drive resume %s", d.id, a, d.id)
	}
	return driveState("drive %s is held by session %s", d.id, a)
}

func (d *drive) bump(out bool, seq int) {
	if out {
		d.info.SeqOut = seq
	} else {
		d.info.SeqIn = seq
	}
	d.save()
}

func driveSend(path string, w *Workspace, args []string) error {
	if len(args) == 0 {
		return driveUsage("usage: mw drive send open --to MEMBER GOAL | task|steer|note ID N TEXT | result ID N pass|fail [TEXT] | stop|close|pause ID")
	}
	o, rest0, err := driveLead(args, "tenant", "session", "to", "id")
	if err != nil {
		return err
	}
	if len(rest0) == 0 {
		return driveUsage("usage: mw drive send [options] EVENT ... (mw help drive)")
	}
	ev := rest0[0]
	o2, pos, err := driveLead(rest0[1:], "tenant", "session", "to", "id")
	if err != nil {
		return err
	}
	for k, v := range o2 {
		o[k] = v
	}
	if ev != "open" && (o["to"] != "" || o["id"] != "") {
		return driveUsage("--to and --id are only for open")
	}
	session, t, err := driveSession(w, o)
	if err != nil {
		return err
	}
	now := nowStamp()
	if ev == "open" {
		return driveSendOpen(path, w, t, session, o, pos, now)
	}
	if !driverEvents[ev] && !remoteEvents[ev] || ev == "busy" || ev == "closed" || ev == "resumed" {
		return driveUsage("drive send: unknown or non-sendable event %q (resume has its own command)", ev)
	}
	if len(pos) == 0 {
		return driveUsage("usage: mw drive send %s ID ...", ev)
	}
	id := pos[0]
	m := driveMsg{ID: id, Event: ev}
	rest := pos[1:] // literal: everything after ID N is TEXT, whatever it looks like
	num := func() error {
		if len(rest) == 0 {
			return driveUsage("usage: mw drive send %s ID N ...", ev)
		}
		n, e := strconv.Atoi(rest[0])
		if e != nil || strconv.Itoa(n) != rest[0] {
			return driveUsage("drive: %q is not a task number", rest[0])
		}
		m.N, rest = n, rest[1:]
		return nil
	}
	switch ev {
	case "task", "steer", "note":
		if err := num(); err != nil {
			return err
		}
		m.Text = strings.Join(rest, " ")
	case "result":
		if err := num(); err != nil {
			return err
		}
		if len(rest) == 0 {
			return driveUsage("usage: mw drive send result ID N pass|fail [TEXT]")
		}
		m.Verdict, m.Text = rest[0], strings.Join(rest[1:], " ")
	case "pause":
		m.Text = "idle"
		if len(rest) != 0 {
			return driveUsage("usage: mw drive send pause ID")
		}
	default: // stop, close
		if len(rest) != 0 {
			return driveUsage("usage: mw drive send %s ID", ev)
		}
	}
	m.Seq = 1
	if err := m.validate(); err != nil { // before the board is read: a bad line is USAGE whatever the state
		return err
	}
	d := findDrive(t, id)
	if d == nil {
		return driveState("no drive %s on tenant %s", id, t.Slug)
	}
	if d.driverSide() && !driverEvents[ev] {
		return driveState("%s is sent by the remote, not the driver", ev)
	}
	if !d.driverSide() && !remoteEvents[ev] {
		return driveState("%s is sent by the driver, not the remote", ev)
	}
	if !d.open() {
		return driveState("drive %s is closed", id)
	}
	if err := d.holder(w, session); err != nil {
		return err
	}
	m.Seq = d.info.SeqOut + 1
	line, err := formatDriveLine(m)
	if err != nil {
		return err
	}
	if err := applyEvent(w, d, m, t.Slug, session, now, true); err != nil {
		return err
	}
	d.bump(true, m.Seq)
	touchSession(w, session, now)
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Println(line)
	return nil
}

func driveSendOpen(path string, w *Workspace, t *Tenant, session string, o map[string]string, pos []string, now string) error {
	to := o["to"]
	if to == "" || !slugPattern.MatchString(to) {
		return driveUsage("usage: mw drive send open --to MEMBER [--id SLUG] GOAL...")
	}
	if to == t.Slug {
		return driveUsage("drive: a member cannot drive itself (--to %s is this tenant)", to)
	}
	goal := strings.Join(pos, " ")
	if err := checkDriveText(goal, true); err != nil {
		return err
	}
	if h := remoteHeld(t, session); h != nil {
		return driveState("session %s is driven by %s (drive %s): a driven session cannot drive", session, h.info.Driver, h.id)
	}
	avail := driveIDMax - len(t.Slug) - 1
	slug := o["id"]
	if slug != "" {
		if !driveSlugRe.MatchString(slug) {
			return driveUsage("drive: --id must be from [a-z0-9.-]")
		}
	} else {
		slug = slugify(goal)
		if avail > 0 && len(slug) > avail {
			slug = strings.TrimRight(slug[:avail], "-")
		}
		if slug == "" {
			slug = "d"
		}
	}
	id := t.Slug + "." + slug
	if o["id"] == "" {
		base := id
		for n := 2; findDrive(t, id) != nil; n++ {
			sfx := "-" + strconv.Itoa(n)
			id = base
			if len(id)+len(sfx) > driveIDMax {
				id = id[:driveIDMax-len(sfx)]
			}
			id += sfx
		}
	}
	if err := checkDriveID(id); err != nil {
		return err
	}
	if findDrive(t, id) != nil {
		return driveState("drive %s already exists on tenant %s", id, t.Slug)
	}
	m := driveMsg{ID: id, Seq: 1, Event: "open", To: to, Text: goal}
	line, err := formatDriveLine(m)
	if err != nil {
		return err
	}
	info := driveInfo{Driver: t.Slug, Remote: to, DriverSession: session, SeqOut: 1}
	story := newDriveItem(t, "story", "drive>"+to+": "+id, "", driveTicketPf+id, info.String(), now)
	claimStory(w, story, session, now)
	driveNote(story, now, session, recordText(m, t.Slug))
	touchSession(w, session, now)
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Println(line)
	return nil
}

func driveRecv(path string, w *Workspace, args []string) error {
	o, pos, err := driveLead(args, "tenant", "session")
	if err != nil {
		return err
	}
	line := strings.Join(pos, " ")
	if len(pos) == 0 || len(pos) == 1 && pos[0] == "-" {
		if line, err = readDriveLine(); err != nil {
			return err
		}
	}
	m, err := parseDriveLine(line)
	if err != nil {
		return err
	}
	session, t, err := driveSession(w, o)
	if err != nil {
		return err
	}
	now := nowStamp()
	sender, _, _ := strings.Cut(m.ID, ".") // the driver is named by the id
	d := findDrive(t, m.ID)
	if d == nil && m.Event == "open" {
		return driveRecvOpen(path, w, t, session, m, sender, now)
	}
	if d == nil {
		return driveState("no drive %s on tenant %s", m.ID, t.Slug)
	}
	peer := d.peer()
	if m.Seq <= d.info.SeqIn {
		fmt.Println("dup")
		return nil
	}
	if m.Seq > d.info.SeqIn+1 {
		return driveState("drive %s: missing seq %d (got %d): deliver the lines in order", m.ID, d.info.SeqIn+1, m.Seq)
	}
	if d.driverSide() && !remoteEvents[m.Event] || !d.driverSide() && !driverEvents[m.Event] {
		return driveState("%s does not come from the %s of drive %s", m.Event, map[bool]string{true: "remote", false: "driver"}[d.driverSide()], m.ID)
	}
	if !d.open() && !(m.Event == "closed" && d.driverSide()) {
		return driveState("drive %s is closed", m.ID)
	}
	if err := d.holder(w, session); err != nil && d.open() {
		return err
	}
	if m.Event == "open" {
		return driveState("drive %s is already open", m.ID)
	}
	if err := applyEvent(w, d, m, peer, session, now, false); err != nil {
		return err
	}
	d.bump(false, m.Seq)
	reply := ""
	if m.Event == "close" { // the remote answers closed
		r := driveMsg{ID: m.ID, Seq: d.info.SeqOut + 1, Event: "closed"}
		line, err := formatDriveLine(r)
		if err != nil {
			return err
		}
		driveNote(d.story, now, session, recordText(r, t.Slug))
		d.bump(true, r.Seq)
		reply = line
	}
	touchSession(w, session, now)
	if err := save(path, w); err != nil {
		return err
	}
	if reply != "" {
		fmt.Println(reply)
	}
	return nil
}

func driveRecvOpen(path string, w *Workspace, t *Tenant, session string, m driveMsg, driver, now string) error {
	if driver == t.Slug {
		return driveState("drive %s names this tenant as the driver", m.ID)
	}
	if m.To != t.Slug {
		return driveState("drive %s is addressed to member %s, not to this tenant (%s)", m.ID, m.To, t.Slug)
	}
	if h := remoteHeld(t, session); h != nil || driverHeld(t, session) != nil {
		if h == nil {
			h = driverHeld(t, session)
		}
		r := driveMsg{ID: m.ID, Seq: 1, Event: "busy", Text: h.peer()}
		line, err := formatDriveLine(r)
		if err != nil {
			return err
		}
		fmt.Println(line)
		return nil
	}
	if m.Seq != 1 {
		return driveState("drive %s: an open line has seq 1", m.ID)
	}
	info := driveInfo{Driver: driver, Remote: t.Slug, RemoteSession: session, SeqIn: 1}
	story := newDriveItem(t, "story", "drive by "+driver+": "+m.ID, "", driveTicketPf+m.ID, info.String(), now)
	claimStory(w, story, session, now)
	driveNote(story, now, session, recordText(m, driver))
	touchSession(w, session, now)
	return save(path, w)
}

func driveResume(path string, w *Workspace, args []string) error {
	o, pos, err := driveOpts(args, "tenant", "session")
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return driveUsage("usage: mw drive resume ID")
	}
	if err := checkDriveID(pos[0]); err != nil {
		return err
	}
	session, t, err := driveSession(w, o)
	if err != nil {
		return err
	}
	d := findDrive(t, pos[0])
	if d == nil {
		return driveState("no drive %s on tenant %s", pos[0], t.Slug)
	}
	if !d.open() {
		return driveState("drive %s is closed", d.id)
	}
	if a := d.story.Assignee; a != "" && a != session {
		if s, e := sessionByID(w, a); e == nil && s.State != "stopped" {
			return driveState("drive %s is held by live session %s", d.id, a)
		}
	}
	if h := remoteHeld(t, session); h != nil && h.id != d.id {
		return driveState("session %s already holds drive %s as a remote", session, h.id)
	}
	if d.driverSide() {
		if h := remoteHeld(t, session); h != nil {
			return driveState("session %s is driven (drive %s): a driven session cannot drive", session, h.id)
		}
	} else if h := driverHeld(t, session); h != nil {
		return driveState("session %s drives %s: a driving session cannot be driven", session, h.id)
	}
	now := nowStamp()
	m := driveMsg{ID: d.id, Seq: d.info.SeqOut + 1, Event: "resumed", Text: session}
	line, err := formatDriveLine(m)
	if err != nil {
		return err
	}
	if d.story.Assignee != session || d.story.Status != "doing" {
		claimStory(w, d.story, session, now)
	}
	if err := applyEvent(w, d, m, t.Slug, session, now, true); err != nil {
		return err
	}
	d.bump(true, m.Seq)
	touchSession(w, session, now)
	if err := save(path, w); err != nil {
		return err
	}
	fmt.Println(line)
	return nil
}

// ---- status and check ----

type driveRow struct {
	ID       string `json:"id"`
	Tenant   string `json:"tenant"`
	Role     string `json:"role"`
	Peer     string `json:"peer"`
	Session  string `json:"session"`
	SeqIn    int    `json:"seq_in"`
	SeqOut   int    `json:"seq_out"`
	Age      string `json:"age"`
	Last     string `json:"last"`
	last     time.Time
	lastDrv  time.Time
	assignee string
}

// driveNotes visits the drive notes of the story and its children: the timestamp and the sender label.
func driveTimes(d *drive) (last, lastDriver time.Time) {
	scan := func(it *Item) {
		for _, n := range it.Notes {
			ts, rest, ok := strings.Cut(n, " ")
			if !ok {
				continue
			}
			if strings.HasPrefix(rest, "[") {
				if i := strings.Index(rest, "] "); i > 0 {
					rest = rest[i+2:]
				}
			}
			if !strings.HasPrefix(rest, driveTag+" ") {
				continue
			}
			at, err := time.Parse(time.RFC3339, ts)
			if err != nil {
				continue
			}
			if at.After(last) {
				last = at
			}
			if m := driveLabelRe.FindStringSubmatch(rest); m != nil && m[1] == d.info.Driver && at.After(lastDriver) {
				lastDriver = at
			}
		}
	}
	scan(d.story)
	for i := range d.t.Items {
		if d.t.Items[i].Parent == d.story.ID {
			scan(&d.t.Items[i])
		}
	}
	return
}

func openDrives(w *Workspace, o map[string]string) ([]driveRow, error) {
	var tenants []*Tenant
	if o["tenant"] != "" || os.Getenv("MW_TENANT") != "" || value(o, "session", os.Getenv("MW_SESSION")) != "" {
		t, err := selectTenantRaw(w, o)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	} else {
		for i := range w.Tenants {
			tenants = append(tenants, &w.Tenants[i])
		}
	}
	now := nowFn()
	rows := []driveRow{}
	for _, t := range tenants {
		for _, d := range drivesOf(t) {
			if !d.open() {
				continue
			}
			role, sess := "remote", d.info.RemoteSession
			if d.driverSide() {
				role, sess = "driver", d.info.DriverSession
			}
			if a := d.story.Assignee; a != "" {
				sess = a
			}
			last, lastDrv := driveTimes(d)
			age, lastStr := "?", ""
			if !last.IsZero() {
				age, lastStr = shortAge(now.Sub(last)), last.Format(time.RFC3339)
			}
			rows = append(rows, driveRow{ID: d.id, Tenant: t.Slug, Role: role, Peer: d.peer(), Session: sess,
				SeqIn: d.info.SeqIn, SeqOut: d.info.SeqOut, Age: age, Last: lastStr, last: last, lastDrv: lastDrv, assignee: d.story.Assignee})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Tenant+rows[i].ID < rows[j].Tenant+rows[j].ID })
	return rows, nil
}

func shortAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "0m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func driveStatus(w *Workspace, args []string) error {
	o, pos, err := driveOpts(args, "tenant", "session")
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return driveUsage("unexpected argument %q", pos[0])
	}
	rows, err := openDrives(w, o)
	if err != nil {
		return err
	}
	if o["json"] == "true" {
		b, _ := json.Marshal(rows)
		fmt.Println(string(b))
		return nil
	}
	for _, r := range rows {
		fmt.Printf("%s  %s  peer=%s  session=%s  in=%d out=%d  last %s ago\n", r.ID, r.Role, r.Peer, dash(r.Session), r.SeqIn, r.SeqOut, r.Age)
	}
	return nil
}

func driveCheck(w *Workspace, args []string) error {
	o, pos, err := driveOpts(args, "tenant", "session", "idle", "at")
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return driveUsage("unexpected argument %q", pos[0])
	}
	if o["idle"] == "" {
		return driveUsage("usage: mw drive check --idle DUR [--at RFC3339] [-j]")
	}
	session, _, err := driveSession(w, o)
	if err != nil {
		return err
	}
	limit, err := parseAge(o["idle"])
	if err != nil {
		return driveUsage("%v", err)
	}
	if at := o["at"]; at != "" {
		ts, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return driveUsage("--at must be an RFC3339 time, for example 2026-11-07T12:00:00Z")
		}
		old := nowFn
		nowFn = func() time.Time { return ts.UTC() }
		defer func() { nowFn = old }()
	}
	rows, err := openDrives(w, o)
	if err != nil {
		return err
	}
	now := nowFn()
	idle := []driveRow{}
	for _, r := range rows {
		if r.Role != "remote" || r.assignee != session { // only the remote session that holds the drive pauses it
			continue
		}
		ref := r.lastDrv
		if ref.IsZero() {
			ref = r.last
		}
		if !ref.IsZero() && now.Sub(ref) > limit {
			r.Age = shortAge(now.Sub(ref))
			idle = append(idle, r)
		}
	}
	if o["json"] == "true" {
		b, _ := json.Marshal(idle)
		fmt.Println(string(b))
		return nil
	}
	for _, r := range idle {
		fmt.Printf("idle %s  %s  peer=%s  no driver event for %s\n", r.ID, r.Role, r.Peer, r.Age)
	}
	return nil
}

// readDriveLine reads the line of `drive recv` from stdin (no shell quoting of a peer's TEXT is involved).
func readDriveLine() (string, error) {
	if driveIn == io.Reader(os.Stdin) && realStdinIsTerminal() {
		return "", driveUsage("usage: mw drive recv [LINE] (without LINE the line is read from stdin: pipe it or use a quoted heredoc)")
	}
	b, err := io.ReadAll(io.LimitReader(driveIn, 4096+1))
	if err != nil {
		return "", driveUsage("drive recv: cannot read stdin: %v", err)
	}
	if len(b) > 4096 {
		return "", driveUsage("drive recv: line is over 4096 bytes")
	}
	line := strings.TrimRight(string(b), "\r\n")
	if strings.TrimSpace(line) == "" {
		return "", driveUsage("usage: mw drive recv [LINE] (no line on stdin)")
	}
	return line, nil
}
