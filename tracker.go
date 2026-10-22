package main

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Tracker tickets (optional, off by default): a job may name one issue tracker (job keys tracker and
// tracker_url) and an item may carry a ticket from it. mw only validates and builds links from fixed templates;
// it never calls the tracker and never reads a response. See docs/specs/news-model.md.

var trackerKinds = []string{"jira", "linear", "youtrack", "github", "gitlab", "azure", "shortcut", "clickup", "asana"}

var (
	ticketKeyRe   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,9}-[0-9]{1,9}$`)
	ticketNumRe   = regexp.MustCompile(`^[0-9]{1,9}$`)
	ticketAzureRe = regexp.MustCompile(`^(?:AB#)?([0-9]{1,9})$`)
	ticketScRe    = regexp.MustCompile(`^(?:sc-)?([0-9]{1,9})$`)
	ticketClickRe = regexp.MustCompile(`^#?([a-z0-9]{6,12})$`)
	ticketAsanaRe = regexp.MustCompile(`^(?:asana )?([0-9]{10,20})$`)
	ticketPlainRe = regexp.MustCompile(`[^A-Za-z0-9#_.-]`)
	trackerURLRe  = regexp.MustCompile(`^https://[A-Za-z0-9.-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~%-]+)*/?$`)
)

func knownTracker(kind string) bool {
	for _, k := range trackerKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// canonTicket validates raw against the tracker kind and returns the canonical form ("Shown as").
func canonTicket(kind, raw string) (string, error) {
	s := strings.TrimSpace(raw)
	bad := func() (string, error) {
		return "", fmt.Errorf("ticket %q is not a valid %s ticket (%s)", compactText(raw, 40), kind, trackerExample(kind))
	}
	switch kind {
	case "jira", "linear", "youtrack":
		if u := strings.ToUpper(s); ticketKeyRe.MatchString(u) {
			return u, nil
		}
	case "github", "gitlab":
		if n := strings.TrimPrefix(s, "#"); ticketNumRe.MatchString(n) {
			return "#" + n, nil
		}
	case "azure":
		if m := ticketAzureRe.FindStringSubmatch(strings.ToUpper(s)); m != nil {
			return "AB#" + m[1], nil
		}
	case "shortcut":
		if m := ticketScRe.FindStringSubmatch(strings.ToLower(s)); m != nil {
			return "sc-" + m[1], nil
		}
	case "clickup":
		if m := ticketClickRe.FindStringSubmatch(strings.ToLower(s)); m != nil {
			return "#" + m[1], nil
		}
	case "asana":
		if m := ticketAsanaRe.FindStringSubmatch(strings.ToLower(s)); m != nil {
			return "Asana " + m[1], nil
		}
	default: // a tracker kind from a newer mw: kept, plain text
		if s != "" && len(s) <= 40 && !ticketPlainRe.MatchString(s) {
			return s, nil
		}
	}
	return bad()
}

func trackerExample(kind string) string {
	return map[string]string{"jira": "like MOB-142", "linear": "like ENG-42", "youtrack": "like APP-7", "github": "123 or #123", "gitlab": "123 or #123",
		"azure": "123 or AB#123", "shortcut": "123 or sc-123", "clickup": "6 to 12 letters or digits, like 86abc12xy", "asana": "10 to 20 digits"}[kind]
}

// validTrackerURL checks a tracker base URL: https only, no query, fragment or credentials, trailing slash trimmed.
func validTrackerURL(s string) (string, error) {
	if len(s) > 200 || !trackerURLRe.MatchString(s) {
		return "", errors.New("must be an https URL with a host and an optional path, no query, fragment, credentials or special characters")
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.Host == "" {
		return "", errors.New("must be an https URL without query, fragment or credentials")
	}
	return strings.TrimSuffix(s, "/"), nil
}

// trackerLink builds the link of a canonical ticket from the fixed template; "" when it cannot be built
// (unknown kind, no usable base URL). ClickUp and Asana use their fixed hosts.
func trackerLink(kind, base, canon string) string {
	if !knownTracker(kind) {
		return ""
	}
	switch kind {
	case "clickup":
		return "https://app.clickup.com/t/" + strings.TrimPrefix(canon, "#")
	case "asana":
		return "https://app.asana.com/0/0/" + strings.TrimPrefix(canon, "Asana ")
	}
	b, err := validTrackerURL(base)
	if err != nil {
		return ""
	}
	switch kind {
	case "jira":
		return b + "/browse/" + canon
	case "linear", "youtrack":
		return b + "/issue/" + canon
	case "github":
		return b + "/issues/" + canon[1:]
	case "gitlab":
		return b + "/-/issues/" + canon[1:]
	case "azure":
		return b + "/_workitems/edit/" + canon[3:]
	case "shortcut":
		return b + "/story/" + canon[3:]
	}
	return ""
}

// ticketShown returns the ticket, its label and link as the model carries them. A ticket that does not
// validate under the job's current tracker (an unknown kind, an edited file) stays as plain text: a
// sanitized label and no link.
func ticketShown(cfg JobConfig, raw string) (ticket, label, link string) {
	if raw == "" {
		return "", "", ""
	}
	if cfg.Tracker != "" && knownTracker(cfg.Tracker) {
		if c, err := canonTicket(cfg.Tracker, raw); err == nil {
			return c, c, trackerLink(cfg.Tracker, cfg.TrackerURL, c)
		}
	}
	p := ticketPlainRe.ReplaceAllString(raw, "")
	if len(p) > 40 {
		p = p[:40]
	}
	return p, p, ""
}

// ticketForItem validates a ticket for an item of tenant t: refused when the job has no tracker.
func ticketForItem(w *Workspace, t *Tenant, raw string) (string, error) {
	cfg, ok := w.Jobs[t.Job]
	if t.Job == "" || !ok || cfg.Tracker == "" {
		return "", errors.New("ticket needs a tracker on the item's job: mw jobs set JOB tracker=KIND [tracker_url=URL] (kinds: " + strings.Join(trackerKinds, ", ") + ")")
	}
	return canonTicket(cfg.Tracker, raw)
}
