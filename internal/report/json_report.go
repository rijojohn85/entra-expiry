package report

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/rijojohn85/entra-expiry/internal/provider"
)

// JSONReport renders the machine-readable payload. The embedded Summary
// supplies the top-level counts and their json tags; the credential rows
// are the same sorted Rows the table consumes.
type JSONReport struct {
	Summary
	Credentials []Row `json:"credentials"`
	writer      io.Writer
}

func NewJSONReport(apps []provider.Application, days int, w io.Writer) *JSONReport {
	rows, s := Build(apps, days, time.Now())
	return &JSONReport{Summary: s, Credentials: rows, writer: w}
}

func (r *JSONReport) Generate() error {
	enc := json.NewEncoder(r.writer)
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("encode json report: %w", err)
	}
	return nil
}
