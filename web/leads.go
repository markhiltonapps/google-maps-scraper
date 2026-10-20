package web

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/gosom/google-maps-scraper/gmaps"
)

// leadsFormat is the download format that returns a short, human-friendly
// spreadsheet instead of the full scrape output.
const leadsFormat = "leads"

// Display values used in the leads spreadsheet.
const (
	leadsYes = "Yes"
	leadsNo  = "No"
)

type leadsColumn struct {
	header string
	value  func(row map[string]string) string
	// aiOnly columns are included only when AI tools detection ran.
	aiOnly bool
}

var leadsColumns = []leadsColumn{
	{header: "Business Name", value: field("title")},
	{header: "Category", value: field("category")},
	{header: "Phone", value: field("phone")},
	{header: "Has Website", value: func(row map[string]string) string {
		if row["website"] == "" {
			return leadsNo
		}

		return leadsYes
	}},
	{header: "Website", value: func(row map[string]string) string { return stripTrackingParams(row["website"]) }},
	{header: "Email", value: func(row map[string]string) string { return cleanEmailList(row["emails"]) }},
	{header: "AI Chat on Website", value: func(row map[string]string) string { return friendlyAIChat(row["ai_chat"]) }, aiOnly: true},
	{header: "Chat Tool", value: field("chat_widgets"), aiOnly: true},
	{header: "AI Phone Clues", value: field("ai_phone_signals"), aiOnly: true},
	{header: "Rating", value: func(row map[string]string) string { return trimFloat(row["review_rating"]) }},
	{header: "Reviews", value: field("review_count")},
	{header: "Address", value: field("address")},
	{header: "Google Maps Link", value: field("link")},
}

func field(name string) func(map[string]string) string {
	return func(row map[string]string) string { return row[name] }
}

var friendlyAIChatValues = map[string]string{
	"yes":         leadsYes,
	"possible":    "Possibly (chat tool with optional AI)",
	"no":          leadsNo,
	"unknown":     "Couldn't check (website didn't load)",
	"no website":  "No website",
	"not checked": "Not checked (social media page)",
}

func friendlyAIChat(v string) string {
	if friendly, ok := friendlyAIChatValues[v]; ok {
		return friendly
	}

	return v
}

// cleanEmailList tidies the emails column of results scraped before
// gmaps.CleanEmails existed, so older jobs read cleanly too.
func cleanEmailList(v string) string {
	if v == "" {
		return ""
	}

	return strings.Join(gmaps.CleanEmails(strings.Split(v, ",")), ", ")
}

// trimFloat turns "4.900000" into "4.9".
func trimFloat(v string) string {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return v
	}

	return strconv.FormatFloat(f, 'f', -1, 64)
}

// stripTrackingParams removes utm_* parameters Google Business Profile links
// often carry, so the website column shows the address a person would type.
func stripTrackingParams(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery == "" {
		return raw
	}

	q := u.Query()
	for key := range q {
		if strings.HasPrefix(strings.ToLower(key), "utm_") {
			q.Del(key)
		}
	}

	u.RawQuery = q.Encode()

	return u.String()
}

// leadsTable is a results CSV converted to the short leads format.
type leadsTable struct {
	Headers []string
	Rows    [][]string
	// Total counts every row, including any beyond the row limit.
	Total int
}

// Truncated reports whether rows were left out because of the row limit.
func (t leadsTable) Truncated() bool {
	return len(t.Rows) < t.Total
}

// buildLeads converts a full results CSV into the short leads format,
// keeping at most maxRows rows (all rows when maxRows <= 0).
func buildLeads(src io.Reader, maxRows int) (leadsTable, error) {
	reader := csv.NewReader(src)

	headers, err := reader.Read()
	if errors.Is(err, io.EOF) {
		headers = nil
	} else if err != nil {
		return leadsTable{}, fmt.Errorf("read header: %w", err)
	}

	hasAIColumns := false

	for _, h := range headers {
		if h == "ai_chat" {
			hasAIColumns = true
		}
	}

	var columns []leadsColumn

	for _, c := range leadsColumns {
		if c.aiOnly && !hasAIColumns {
			continue
		}

		columns = append(columns, c)
	}

	table := leadsTable{Headers: make([]string, len(columns))}
	for i, c := range columns {
		table.Headers[i] = c.header
	}

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return leadsTable{}, fmt.Errorf("read row: %w", err)
		}

		table.Total++

		if maxRows > 0 && len(table.Rows) >= maxRows {
			continue
		}

		row := make(map[string]string, len(headers))
		for i, h := range headers {
			if i < len(record) {
				row[h] = record[i]
			}
		}

		out := make([]string, len(columns))
		for i, c := range columns {
			out[i] = c.value(row)
		}

		table.Rows = append(table.Rows, out)
	}

	return table, nil
}

// writeLeadsCSV converts a full results CSV into the short leads format.
func writeLeadsCSV(dst io.Writer, src io.Reader) error {
	table, err := buildLeads(src, 0)
	if err != nil {
		return err
	}

	writer := csv.NewWriter(dst)

	if err := writer.Write(table.Headers); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	if err := writer.WriteAll(table.Rows); err != nil {
		return fmt.Errorf("write rows: %w", err)
	}

	return writer.Error()
}
