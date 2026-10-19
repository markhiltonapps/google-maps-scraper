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

	for i := range want {
		if strings.Join(rows[i], "|") != strings.Join(want[i], "|") {
			t.Fatalf("row %d:\n got  %q\n want %q", i, rows[i], want[i])
		}
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
