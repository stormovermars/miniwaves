package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrackerTicketsPerKind(t *testing.T) {
	cases := []struct {
		kind, ok, canon, bad, base, link string
	}{
		{"jira", "mob-142", "MOB-142", "MOB142", "https://acme.atlassian.net/", "https://acme.atlassian.net/browse/MOB-142"},
		{"linear", "ENG-42", "ENG-42", "42", "https://linear.app/acme", "https://linear.app/acme/issue/ENG-42"},
		{"youtrack", "APP-7", "APP-7", "APP-", "https://acme.youtrack.cloud", "https://acme.youtrack.cloud/issue/APP-7"},
		{"github", "#123", "#123", "12a", "https://github.com/acme/app", "https://github.com/acme/app/issues/123"},
		{"gitlab", "123", "#123", "##1", "https://gitlab.com/acme/app", "https://gitlab.com/acme/app/-/issues/123"},
		{"azure", "AB#123", "AB#123", "XY#1", "https://dev.azure.com/acme/App", "https://dev.azure.com/acme/App/_workitems/edit/123"},
		{"shortcut", "123", "sc-123", "sc-", "https://app.shortcut.com/acme", "https://app.shortcut.com/acme/story/123"},
		{"clickup", "86abc12xy", "#86abc12xy", "ab", "", "https://app.clickup.com/t/86abc12xy"},
		{"asana", "1203456789012345", "Asana 1203456789012345", "123", "", "https://app.asana.com/0/0/1203456789012345"},
	}
	for _, c := range cases {
		got, err := canonTicket(c.kind, c.ok)
		if err != nil || got != c.canon {
			t.Errorf("%s: %q -> %q %v, want %q", c.kind, c.ok, got, err, c.canon)
		}
		if _, err := canonTicket(c.kind, c.bad); err == nil {
			t.Errorf("%s: %q accepted", c.kind, c.bad)
		}
		if l := trackerLink(c.kind, c.base, c.canon); l != c.link {
			t.Errorf("%s: link %q, want %q", c.kind, l, c.link)
		}
	}
	// the fixed hosts ignore tracker_url; the others need one
	if trackerLink("clickup", "", "#86abc12xy") == "" || trackerLink("asana", "", "Asana 1203456789012345") == "" {
		t.Error("fixed host kinds need no base")
	}
	if trackerLink("jira", "", "MOB-1") != "" || trackerLink("future", "https://x.com", "A-1") != "" {
		t.Error("a link without a usable base or with an unknown kind")
	}
	for _, in := range []string{"mob-0", "A-1234567890", "1-2"} {
		if _, err := canonTicket("jira", in); err == nil && in != "mob-0" {
			t.Errorf("jira accepted %q", in)
		}
	}
}

func TestTrackerURLValidation(t *testing.T) {
	for _, bad := range []string{"http://acme.atlassian.net", "https://a.com?x=1", "https://a.com/#f", "https://u:p@a.com", "https://a.com/a b", "javascript:alert(1)", "https://", "ftp://a.com", "https://a.com/(x)", "https://a.com/<x>", "https://a.com/?"} {
		if _, err := validTrackerURL(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	inTempDir(t)
	runAll(t, []string{"init", "T"}, []string{"jobs", "enable", "j", "-t", "t"})
	wantErr(t, "tracker_url", "jobs", "set", "j", "tracker_url=http://x.com")
	wantErr(t, "tracker_url", "jobs", "set", "j", "tracker_url=https://u:p@x.com")
	wantErr(t, "tracker:", "jobs", "set", "j", "tracker=trello")
	runAll(t, []string{"jobs", "set", "j", "tracker=jira", "tracker_url=https://acme.atlassian.net/"})
	out := captureOutput(t, func() { runAll(t, []string{"jobs", "show", "j", "-j"}) })
	if !strings.Contains(out, `"tracker_url": "https://acme.atlassian.net"`) || !strings.Contains(out, `"tracker": "jira"`) {
		t.Fatalf("show:\n%s", out)
	}
	runAll(t, []string{"jobs", "set", "j", "tracker=", "tracker_url="})
	if out = captureOutput(t, func() { runAll(t, []string{"jobs", "show", "j", "-j"}) }); strings.Contains(out, "tracker") {
		t.Fatalf("a job without a tracker lists tracker keys:\n%s", out)
	}
}

func TestTicketCommands(t *testing.T) {
	inTempDir(t)
	t.Setenv("MW_SESSION", "")
	runAll(t, []string{"init", "T"}, []string{"gate", "level", "trust", "--workspace", "--by", "t"}, []string{"jobs", "enable", "j", "-t", "t"})
	wantErr(t, "ticket needs a tracker", "add", "task", "X", "--ticket", "MOB-1")
	runAll(t, []string{"jobs", "set", "j", "tracker=jira", "tracker_url=https://acme.atlassian.net"})
	wantErr(t, "not a valid jira ticket", "add", "task", "X", "--ticket", "nope")
	wantErr(t, "not a valid jira ticket", "add", "task", "X", "--ticket", "MOB-1; rm -rf")
	runAll(t, []string{"add", "task", "TSK-011 looks like an id", "--ticket", "mob-142"}, []string{"add", "task", "Plain"})
	wantErr(t, "not a valid jira ticket", "edit", "TSK-002", "ticket=bad key")
	runAll(t, []string{"edit", "TSK-002", "ticket=ENG-9"})
	for _, args := range [][]string{{"list", "-j"}, {"list", "-j", "-v"}, {"show", "TSK-001", "-j"}} {
		out := captureOutput(t, func() { runAll(t, args) })
		if !strings.Contains(out, `"ticket": "MOB-142"`) {
			t.Errorf("%v lacks the ticket:\n%s", args, out)
		}
	}
	runAll(t, []string{"edit", "TSK-002", "ticket="})
	if out := captureOutput(t, func() { runAll(t, []string{"show", "TSK-002", "-j"}) }); strings.Contains(out, `"ticket"`) {
		t.Fatalf("ticket not cleared:\n%s", out)
	}
	// a wave cannot carry one; an unknown edit argument is refused
	runAll(t, []string{"wave", "create", "W"})
	wantErr(t, "milestone cannot carry", "edit", "WAV-001", "ticket=MOB-1")
	wantErr(t, "only ticket=KEY", "edit", "TSK-001", "title=x")
}

func TestItemUnknownFieldsSurvive(t *testing.T) {
	path := inTempDir(t)
	runAll(t, []string{"init", "T"}, []string{"add", "task", "A"})
	w, _ := load(path)
	p := tenantPath(path, w.Tenants[0].Slug)
	b := mustRead(t, p)
	b = strings.Replace(b, `"kind":"task"`, `"future_field":{"a":[1,2]},"kind":"task"`, 1)
	if !strings.Contains(b, "future_field") {
		t.Fatalf("no kind key in %s", b)
	}
	_ = os.WriteFile(p, []byte(b), 0o644)
	runAll(t, []string{"note", "TSK-001", "hi"})
	var doc struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(mustRead(t, p)), &doc); err != nil || string(doc.Items[0]["future_field"]) != `{"a":[1,2]}` {
		t.Fatalf("unknown item field lost: %v %v", err, doc.Items)
	}
}

func ticketBoard(t *testing.T, kind, base string) (path string) {
	path = newsBoard(t)
	args := []string{"jobs", "set", "tiny", "tracker=" + kind}
	if base != "" {
		args = append(args, "tracker_url="+base)
	}
	runAll(t, args)
	return path
}

// Tickets are optional: setting a tracker on a board without tickets changes no output.
func TestNoTicketsNoChange(t *testing.T) {
	newsBoard(t)
	render := func() []string {
		return []string{newsOut(t, "--job", "tiny", "--format", "json"), newsOut(t, "--job", "tiny", "-o", "-"),
			newsOut(t, "--job", "tiny", "--format", "html", "-o", "-"), newsOut(t, "--job", "tiny", "--format", "text")}
	}
	before := render()
	if strings.Contains(before[0], "ticket") {
		t.Fatal("ticket keys in the model of a board without tickets")
	}
	checkGolden(t, "news-json", before[0]) // the same golden as the board without any tracker
	runAll(t, []string{"jobs", "set", "tiny", "tracker=jira", "tracker_url=https://acme.atlassian.net"})
	for i, got := range render() {
		if got != before[i] {
			t.Errorf("output %d changed after setting a tracker", i)
		}
	}
	if !strings.Contains(before[0], `"id": "TSK-001"`) {
		t.Fatal("mw id missing from the model")
	}
}

func TestTicketsAcrossFormats(t *testing.T) {
	ticketBoard(t, "github", "https://github.com/acme/app")
	runAll(t, []string{"edit", "TSK-002", "ticket=#12", "-t", "news-app"}, []string{"edit", "TSK-003", "ticket=7", "-t", "news-app"})
	m := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	var tickets []newsItem
	for _, it := range append(append(append([]newsItem{}, m.Next.Ready...), m.Now.Blocked...), m.Now.Doing...) {
		if it.Ticket != "" {
			tickets = append(tickets, it)
		}
	}
	if len(tickets) != 2 {
		t.Fatalf("model tickets: %+v", tickets)
	}
	for _, it := range tickets {
		if it.ID == "" || it.Tenant == "" || it.TicketLabel != it.Ticket || !strings.HasPrefix(it.TicketURL, "https://github.com/acme/app/issues/") {
			t.Fatalf("ticket item: %+v", it)
		}
	}
	txt := newsOut(t, "--job", "tiny", "--format", "text")
	md := newsOut(t, "--job", "tiny", "-o", "-")
	htm := newsOut(t, "--job", "tiny", "--format", "html", "-o", "-")
	for _, it := range tickets {
		tag := "[" + it.TicketLabel + "]"
		if !strings.Contains(txt, tag) || strings.Contains(txt, "http") && strings.Contains(txt, it.TicketURL) {
			t.Errorf("text: %s", tag)
		}
		if !strings.Contains(md, tag) || !strings.Contains(md, "- ["+mdLabel(it.TicketLabel)+"](<"+it.TicketURL+">)") {
			t.Errorf("md lacks the link for %s:\n%s", tag, md)
		}
		if !strings.Contains(htm, `<a href="`+it.TicketURL+`">`+tag+`</a>`) {
			t.Errorf("html lacks the link for %s", tag)
		}
	}
	if strings.Count(txt, "[#") != 2 || strings.Count(htm, "[#") != 2 || strings.Count(md, "\n- [") != 2 {
		t.Errorf("counts: text %d html %d md links %d", strings.Count(txt, "[#"), strings.Count(htm, "[#"), strings.Count(md, "\n- ["))
	}
	if w, l := maxWidth(txt); w > 70 {
		t.Errorf("text line of %d cells: %q", w, l)
	}
	if w, l := maxWidth(htm); w > 70 {
		t.Errorf("html line of %d cells: %q", w, l)
	}
}

func TestTicketEscapingAndUnknownTracker(t *testing.T) {
	path := ticketBoard(t, "jira", "https://acme.atlassian.net")
	runAll(t, []string{"edit", "TSK-002", "ticket=MOB_1-5", "-t", "news-app"})
	// a tracker kind from a newer mw, and a hand-edited hostile ticket: plain text, sanitized, no link
	w, _ := load(path)
	cfg := w.Jobs["tiny"]
	cfg.Tracker = "futuretracker"
	w.Jobs["tiny"] = cfg
	for i := range w.Tenants {
		for j := range w.Tenants[i].Items {
			if w.Tenants[i].Items[j].ID == "TSK-003" && w.Tenants[i].Slug == "news-app" {
				w.Tenants[i].Items[j].Ticket = `<script>"x"</script>[a](http://e) ` + "\x1b[31m"
			}
		}
	}
	if err := save(path, w); err != nil {
		t.Fatal(err)
	}
	m := decodeModel(t, newsOut(t, "--job", "tiny", "--format", "json"))
	for _, it := range append(append([]newsItem{}, m.Next.Ready...), m.Now.Blocked...) {
		if it.Ticket != "" && it.TicketURL != "" {
			t.Errorf("a link for an unknown tracker: %+v", it)
		}
		if strings.ContainsAny(it.TicketLabel, "<>\"[]()\x1b ") {
			t.Errorf("unsafe label %q", it.TicketLabel)
		}
	}
	for name, out := range map[string]string{"md": newsOut(t, "--job", "tiny", "-o", "-"), "html": newsOut(t, "--job", "tiny", "--format", "html", "-o", "-")} {
		if strings.Contains(out, "\x1b") || strings.Contains(out, "](http://e)") || strings.Contains(out, `"x"`) {
			t.Errorf("%s not safe:\n%s", name, out)
		}
	}
	// a known tracker: the underscore label is escaped in md and the link is built by mw
	cfg.Tracker = "jira"
	w.Jobs["tiny"] = cfg
	_ = save(path, w)
	md := newsOut(t, "--job", "tiny", "-o", "-")
	if !strings.Contains(md, `[MOB\_1\-5](<https://acme.atlassian.net/browse/MOB_1-5>)`) {
		t.Fatalf("md link:\n%s", md)
	}
	// a hostile url in a hand-edited model never becomes a link
	bad := &newsModel{Version: 1}
	bad.Now.Needs = []newsItem{{Tenant: "t", ID: "A-1", Title: "x", Ticket: "A-1", TicketLabel: "A-1", TicketURL: "https://e.com/x)](javascript:alert(1)"}}
	if out := renderBulletinMD(bad, nil); strings.Contains(out, "javascript") {
		t.Errorf("hostile url rendered:\n%s", out)
	}
	if out := renderBulletinHTML(bad, nil, ""); strings.Contains(out, "<a href=\"https://e.com/x)") {
		t.Errorf("hostile url linked in html")
	}
	_ = filepath.Join
}
