package main

// burndownDoc is the tally behind the board header (there is no mw burndown
// command). Done counts every done item,
// Judged those of them that count as finished (with the judge gate off all
// done items do), Percent = Judged / Total.
type burndownDoc struct {
	Tenant  string `json:"tenant"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Judged  int    `json:"judged"`
	Doing   int    `json:"doing"`
	Ready   int    `json:"ready"`
	Blocked int    `json:"blocked"`
	Percent int    `json:"percent"`
}

// burndownOf is the single burndown rule; the board header uses it too.
func burndownOf(t *Tenant, judgeOn bool) burndownDoc {
	d := burndownDoc{Tenant: t.Slug}
	for _, it := range t.Items {
		if it.Kind == "wave" {
			continue
		}
		d.Total++
		switch it.Status {
		case "doing":
			d.Doing++
		case "ready":
			d.Ready++
		case "blocked":
			d.Blocked++
		}
		if it.Status == "done" {
			d.Done++
		}
		if countsDone(t, it, judgeOn) {
			d.Judged++
		}
	}
	// done items moved out by mw archive still count as finished work
	d.Total += t.Archived
	d.Done += t.Archived
	d.Judged += t.Archived
	if d.Total > 0 {
		d.Percent = d.Judged * 100 / d.Total
	}
	return d
}
