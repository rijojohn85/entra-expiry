package report

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/rijojohn85/entra-expiry/internal/provider"
)

// cred is one credential in the --json payload. The fields are exported
// so encoding/json can see them; time.Time marshals as RFC3339
// ("2026-11-24T05:27:13Z"), the format parsers and monitoring systems expect.
type cred struct {
	App          string    `json:"app"`
	Kind         string    `json:"kind"`
	Name         string    `json:"name"`
	Expires      time.Time `json:"expires"`
	Status       string    `json:"status"`
	DaysToExpire int       `json:"days_to_expire"`
}

// JSONReport renders the machine-readable payload. The embedded summary
// supplies the top-level counts and their json tags.
type JSONReport struct {
	summary
	Credentials []cred `json:"credentials"`
	writer      io.Writer
}

func NewJSONReport(apps []provider.Application, days int, w io.Writer) *JSONReport {
	rows, s := build(apps, days, time.Now())
	jr := &JSONReport{summary: s, writer: w}
	jr.Credentials = make([]cred, 0, len(rows))
	for _, row := range rows {
		jr.Credentials = append(jr.Credentials, cred{
			App:          row.app,
			Kind:         row.kind,
			Name:         row.name,
			Expires:      row.expires,
			Status:       row.status,
			DaysToExpire: row.in,
		})
	}
	return jr
}

func (r *JSONReport) Generate() error {
	enc := json.NewEncoder(r.writer)
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("encode json report: %w", err)
	}
	return nil
}
