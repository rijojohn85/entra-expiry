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

// Report holds the rendered table plus the counts behind it, so future
// consumers (--json, serve metrics) can read the numbers without
// re-scanning.
type Report struct {
	Table  table.Writer
	Writer io.Writer

	Expired    int
	Expiring   int
	Valid      int
	TotalApps  int // all apps scanned, including ones with no credentials
	TotalCreds int // rows actually printed
}

type row struct {
	app     string
	kind    string
	name    string
	expires time.Time
	in      int
	status  string
	rank    int // 0 expired < 1 expiring < 2 valid — sort key
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

func NewReport(apps []provider.Application, days int, output io.Writer) Report {
	writer := table.NewWriter()
	writer.AppendHeader(table.Row{"AppName", "Cred Kind", "Name", "Expires", "IN(days)", "STATUS"})
	now := time.Now()

	r := Report{Table: writer, Writer: output, TotalApps: len(apps)}
	var rows []row
	for _, app := range apps {
		for _, cred := range app.Credentials {
			status, rank := classify(cred.Expires, now, days)
			switch rank {
			case 0:
				r.Expired++
			case 1:
				r.Expiring++
			default:
				r.Valid++
			}
			r.TotalCreds++
			rows = append(rows, row{
				app:     app.DisplayName,
				kind:    cred.Type.String(),
				name:    cred.Name,
				expires: cred.Expires,
				in:      int(cred.Expires.Sub(now).Hours() / 24),
				status:  status,
				rank:    rank,
			})
		}
	}

	// Worst first: expired before expiring before valid, oldest first within.
	slices.SortFunc(rows, func(a, b row) int {
		if c := cmp.Compare(a.rank, b.rank); c != 0 {
			return c
		}
		return a.expires.Compare(b.expires)
	})
	for _, row := range rows {
		writer.AppendRow(table.Row{row.app, row.kind, row.name, row.expires, row.in, row.status})
		writer.AppendSeparator()
	}
	return r
}

func (r Report) Generate() error {
	if _, err := fmt.Fprintln(r.Writer, r.Table.Render()); err != nil {
		return fmt.Errorf("render table: %w", err)
	}
	summary := fmt.Sprintf("\n%d application(s) scanned, %d credential(s): %d expired, %d expiring soon, %d valid\n",
		r.TotalApps, r.TotalCreds, r.Expired, r.Expiring, r.Valid)
	if _, err := fmt.Fprint(r.Writer, summary); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}
