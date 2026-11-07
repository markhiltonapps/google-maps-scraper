package web

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func TestWriteLeadsCSV(t *testing.T) {
	full := "title,category,phone,website,emails,review_rating,review_count,address,link,user_reviews,ai_chat,chat_widgets,ai_phone_signals\n" +
		"Dental Smiles,Dentist,(512) 555-0100,https://mydentalsmiles.com/?utm_source=gbp&page=1,hi@example.com,4.900000,120,1 Main St,https://maps/1,\"[huge]\",yes,RevenueWell AI Chat,\n" +
		"No Site Dental,Dentist,(512) 555-0101,,,4.1,8,2 Main St,https://maps/2,,no website,,\n"

	var out bytes.Buffer
	if err := writeLeadsCSV(&out, strings.NewReader(full)); err != nil {
		t.Fatal(err)
	}

	rows, err := csv.NewReader(&out).ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"Business Name", "Category", "Phone", "Has Website", "Website", "Email", "AI Chat on Website", "Chat Tool", "AI Phone Clues", "Rating", "Reviews", "Address", "Google Maps Link"},
		{"Dental Smiles", "Dentist", "(512) 555-0100", "Yes", "https://mydentalsmiles.com/?page=1", "hi@example.com", "Yes", "RevenueWell AI Chat", "", "4.9", "120", "1 Main St", "https://maps/1"},
		{"No Site Dental", "Dentist", "(512) 555-0101", "No", "", "", "No website", "", "", "4.1", "8", "2 Main St", "https://maps/2"},
	}

	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %v", len(rows), len(want), rows)
	}

	// The website status and prompt are the last two columns; check them apart.
	last := len(want[0])
	for i := range want {
		if len(rows[i]) != last+2 {
			t.Fatalf("row %d has %d columns, want %d", i, len(rows[i]), last+2)
		}

		if strings.Join(rows[i][:last], "|") != strings.Join(want[i], "|") {
			t.Fatalf("row %d:\n got  %q\n want %q", i, rows[i][:last], want[i])
		}
	}

	if rows[0][last] != "Website Status" || rows[0][last+1] != "Website Prompt (copy & paste)" {
		t.Fatalf("unexpected prompt headers: %q", rows[0][last:])
	}

	if rows[1][last] != "Has website" || !strings.Contains(rows[1][last+1], "already has its own website") {
		t.Fatalf("a business with a website must not get a prompt: %q", rows[1][last:])
	}

	if !strings.Contains(rows[2][last+1], "No Site Dental") || !strings.Contains(rows[2][last+1], "Act as an expert web designer") {
		t.Fatalf("the no-website row needs a full prompt: %q", rows[2][last+1])
	}

	if rows[2][last] == "" {
		t.Fatal("expected a status for the no-website row")
	}
}

func TestWriteLeadsCSVOmitsAIColumnsWithoutDetection(t *testing.T) {
	full := "title,phone,website,emails\nAcme,(512) 555-0100,https://acme.example,\n"

	var out bytes.Buffer
	if err := writeLeadsCSV(&out, strings.NewReader(full)); err != nil {
		t.Fatal(err)
	}

	header := strings.SplitN(out.String(), "\n", 2)[0]
	if strings.Contains(header, "AI Chat") {
		t.Fatalf("AI columns must be omitted when detection did not run: %s", header)
	}
}

func TestWriteLeadsCSVEmptyFile(t *testing.T) {
	var out bytes.Buffer
	if err := writeLeadsCSV(&out, strings.NewReader("")); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(out.String(), "Business Name,") {
		t.Fatalf("expected a header row, got %q", out.String())
	}
}

func TestLeadsHasWebsiteValues(t *testing.T) {
	if leadsColumns[hasWebsiteColumn].header != "Has Website" {
		t.Fatalf("hasWebsiteColumn must point at Has Website, got %q", leadsColumns[hasWebsiteColumn].header)
	}

	full := "title,website\nNo Site,\nFacebook Only,https://www.facebook.com/acme\nReal Site,https://acme.com\n"

	table, err := buildLeads(strings.NewReader(full), 0)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{leadsNo, leadsSocialOnly, leadsYes}
	for i, row := range table.Rows {
		if row[hasWebsiteColumn] != want[i] {
			t.Fatalf("row %d Has Website = %q, want %q", i, row[hasWebsiteColumn], want[i])
		}
	}

	if !noOwnSite(table.Rows[0]) || !noOwnSite(table.Rows[1]) || noOwnSite(table.Rows[2]) {
		t.Fatal("noOwnSite must flag the first two rows only")
	}
}
