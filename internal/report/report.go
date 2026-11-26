package report

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rijojohn85/entra-expiry/internal/provider"
)

// Summary is the count data both renderers share — and, thanks to the
// json tags, the top-level shape of the --json payload.
type Summary struct {
	Scanned  int `json:"scanned"` // apps looked at, including ones with no credentials
	Total    int `json:"total"`   // credentials found
	Expired  int `json:"expired"`
	Expiring int `json:"expiring"`
	Valid    int `json:"valid"`
}

type Row struct {
	App     string    `json:"app"`
	Kind    string    `json:"kind"`
	Name    string    `json:"name"`
	Expires time.Time `json:"expires"`
	In      int       `json:"days_to_expire"`
	Status  string    `json:"status"`
}

// classify labels one credential relative to now. A credential expiring
// exactly at the window boundary counts as Valid (Before is strict).
func classify(expires time.Time, now time.Time, days int) (status string, rank int) {
	switch {
	case expires.Before(now):
		return "Expired", 0
	case expires.Before(now.Add(time.Duration(days) * 24 * time.Hour)):
		return "Expiring Soon", 1
	default:
		return "Valid", 2
	}
}

// Build classifies every credential exactly once and sorts worst-first
// (expired before expiring before valid, oldest first within a group).
// Both renderers consume these rows, so the table and the JSON output
// can never disagree.
// ranked pairs a row with its sort key while Build works. The rank never
// escapes Build — consumers see sorted rows only.
type ranked struct {
	row  Row
	rank int // 0 expired < 1 expiring < 2 valid
}

func Build(apps []provider.Application, days int, now time.Time) ([]Row, Summary) {
	var rankedRows []ranked
	var s Summary
	s.Scanned = len(apps)
	for _, app := range apps {
		for _, cred := range app.Credentials {
			status, rank := classify(cred.Expires, now, days)
			switch rank {
			case 0:
				s.Expired++
			case 1:
				s.Expiring++
			case 2:
				s.Valid++
			}
			s.Total++
			rankedRows = append(rankedRows, ranked{
				row: Row{
					App:     app.DisplayName,
					Kind:    cred.Type.String(),
					Name:    cred.Name,
					Expires: cred.Expires,
					In:      int(cred.Expires.Sub(now).Hours() / 24),
					Status:  status,
				},
				rank: rank,
			})
		}
	}
	slices.SortFunc(rankedRows, func(a, b ranked) int {
		if c := cmp.Compare(a.rank, b.rank); c != 0 {
			return c
		}
		return a.row.Expires.Compare(b.row.Expires)
	})
	rows := make([]Row, 0, len(rankedRows))
	for _, rr := range rankedRows {
		rows = append(rows, rr.row)
	}
	return rows, s
}

// Report renders the human-readable table.
type Report struct {
	Table  table.Writer
	Writer io.Writer
	Summary
}

func NewReport(apps []provider.Application, days int, output io.Writer) Report {
	writer := table.NewWriter()
	writer.AppendHeader(table.Row{"AppName", "Cred Kind", "Name", "Expires", "IN(days)", "STATUS"})
	rows, s := Build(apps, days, time.Now())
	for _, row := range rows {
		writer.AppendRow(table.Row{row.App, row.Kind, row.Name, row.Expires, row.In, row.Status})
		writer.AppendSeparator()
	}
	return Report{Table: writer, Writer: output, Summary: s}
}

func (r Report) Generate() error {
	if _, err := fmt.Fprintln(r.Writer, r.Table.Render()); err != nil {
		return fmt.Errorf("render table: %w", err)
	}
	line := fmt.Sprintf("\n%d application(s) scanned, %d credential(s): %d expired, %d expiring soon, %d valid\n",
		r.Scanned, r.Total, r.Expired, r.Expiring, r.Valid)
	if _, err := fmt.Fprint(r.Writer, line); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}
