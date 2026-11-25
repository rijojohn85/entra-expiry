package report

import (
	"fmt"
	"io"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rijojohn85/entra-expiry/internal/provider"
)

type Report struct {
	Table  table.Writer
	Writer io.Writer
}

func NewReport(apps []provider.Application, days int, output io.Writer) Report {
	writer := table.NewWriter()
	now := time.Now()
	writer.AppendHeader(table.Row{"AppName", "Cred Kind", "Name", "Expires", "IN(days)", "STATUS"})
	for _, app := range apps {
		for _, cred := range app.Credentials {
			var status string
			switch {
			case cred.Expires.Before(now):
				status = "Expired"
			case cred.Expires.Before(now.Add(time.Duration(days) * 24 * time.Hour)):
				status = "Expiring Soon"
			default:
				status = "Valid"
			}
			in := int(cred.Expires.Sub(now).Hours() / 24)
			kind := cred.Type.String()
			writer.AppendRow(table.Row{app.DisplayName, kind, cred.Name, cred.Expires, in, status})
			writer.AppendSeparator()
		}
	}
	return Report{Table: writer, Writer: output}
}

func (r Report) Generate() {
	fmt.Fprintln(r.Writer, r.Table.Render())
}
