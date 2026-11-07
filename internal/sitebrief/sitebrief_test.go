//nolint:testpackage // tests unexported cleaning helpers directly
package sitebrief

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanName(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"CJC CONSTRUCTION SERVICES LLC": "CJC Construction Services",
		"Texas Metal Framing, LLC":      "Texas Metal Framing",
		"Mo Builder LLC":                "Mo Builder",
		"Acme Roofing Inc.":             "Acme Roofing",
		"RSL Co":                        "RSL Co",
		"Ec Construction":               "Ec Construction",
		"O'BRIEN ROOFING":               "O'Brien Roofing",
		"  Spaced    Name  ":            "Spaced Name",
		"ABC":                           "ABC",
		"HVAC":                          "HVAC",
		"SMITH & SONS PLUMBING":         "Smith & Sons Plumbing",
	} {
		require.Equal(t, want, cleanName(in), in)
	}
}

func TestNameLooksOff(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, nameLooksOff("Tylers's Premier Construction"))
	require.NotEmpty(t, nameLooksOff("James's Roofing"), "possibly right, so it only asks for a second look")
	require.Empty(t, nameLooksOff("Tyler's Premier Construction"))
	require.Empty(t, nameLooksOff("Acme Roofing"))
}

func TestFormatPhone(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"(346) 301-0145":  "(346) 301-0145",
		"346.301.0145":    "(346) 301-0145",
		"+1 346 301 0145": "(346) 301-0145",
		"1-346-301-0145":  "(346) 301-0145",
		"3463010145":      "(346) 301-0145",
	} {
		got, digits, ok := formatPhone(in)
		require.True(t, ok, in)
		require.Equal(t, want, got, in)
		require.Equal(t, "3463010145", digits, in)
	}

	for _, bad := range []string{"", "555-1234", "12345"} {
		_, digits, ok := formatPhone(bad)
		require.False(t, ok, bad)
		require.Empty(t, digits)
	}
}

func hoursJSON(t *testing.T, byDay map[string]string) string {
	t.Helper()

	m := map[string][]string{}
	for d, h := range byDay {
		m[d] = []string{h}
	}

	b, err := json.Marshal(m)
	require.NoError(t, err)

	return string(b)
}

func TestFormatHours(t *testing.T) {
	t.Parallel()

	// Google puts a narrow no-break space (U+202F) before AM/PM.
	const nnbsp = " "

	week := map[string]string{
		"Monday": "8" + nnbsp + "AM–6" + nnbsp + "PM", "Tuesday": "8" + nnbsp + "AM–6" + nnbsp + "PM",
		"Wednesday": "8" + nnbsp + "AM–6" + nnbsp + "PM", "Thursday": "8" + nnbsp + "AM–6" + nnbsp + "PM",
		"Friday": "8" + nnbsp + "AM–6" + nnbsp + "PM", "Saturday": "9" + nnbsp + "AM–1" + nnbsp + "PM",
		"Sunday": "Closed",
	}

	got, odd := formatHours(hoursJSON(t, week))
	require.False(t, odd)
	require.Equal(t, "Mon–Fri 8 AM–6 PM; Sat 9 AM–1 PM; Sun Closed", got)
	require.NotContains(t, got, nnbsp)

	// A day that opens at 12 AM is a Google typo: leave hours off the page.
	week["Friday"] = "12" + nnbsp + "AM–5" + nnbsp + "PM"
	got, odd = formatHours(hoursJSON(t, week))
	require.True(t, odd)
	require.Empty(t, got)

	// Closing at midnight is normal (restaurants), not suspicious.
	_, odd = formatHours(hoursJSON(t, map[string]string{"Friday": "11 AM–12 AM"}))
	require.False(t, odd)

	for _, empty := range []string{"", "null", "{}", "not json"} {
		got, odd = formatHours(empty)
		require.Empty(t, got, empty)
		require.False(t, odd, empty)
	}
}

func TestReviewerName(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"Dora Craine":              "Dora C.",
		"DIPESH RATILAL":           "Dipesh R.",
		"elizabeth medina":         "Elizabeth M.",
		"Craig “Coach Mac” McEwen": "Craig M.",
		"TJ Stewart":               "TJ S.",
		"Alex":                     "Alex",
		"Mrs. Minton":              "Mrs. Minton",
		"State706 State706":        "Google reviewer",
		"Most Infamous Ent.":       "Google reviewer",
		"CA Travelers Costa Rica":  "Google reviewer",
		"Rebel Renovators":         "Google reviewer",
		"I L":                      "Google reviewer",
		"J. D. L.":                 "Google reviewer",
		"":                         "Google reviewer",
	} {
		require.Equal(t, want, reviewerName(in), in)
	}
}

func rv(name string, rating float64, text, when string) review {
	return review{Name: name, Rating: rating, Description: text, When: when}
}

func TestPickTestimonials(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("They did a careful job and kept the site clean. ", 10)

	reviews := []review{
		rv("Angry Customer", 1, "Terrible work and they never finished the job at all.", "8 years ago"),
		rv("Rating Only", 5, "", "4 years ago"),
		rv("Short One", 5, "Best company", "6 years ago"),
		rv("Phone Poster", 5, "Great work, call me at 281-555-1234 for details.", "a year ago"),
		rv("Competitor Basher", 5, "Much better than the different company we hired first.", "a year ago"),
		rv("Guarantee Claim", 5, "They stand behind the work with a lifetime warranty on everything.", "a year ago"),
		rv("Three Star", 3, "It was fine, nothing special about the whole experience really.", "a year ago"),
		rv("Star Emoji", 5, "⭐⭐⭐⭐⭐  Professional, on time and very easy to work with.", "Edited 3 months ago"),
		rv("Long Writer", 5, long, "2 years ago"),
		rv("good reviewer", 5, "Quick, friendly and the price was fair. Would hire again.", "2 months ago"),
		rv("Shouty Person", 5, "GREAT JOB!! LOVED IT!! BEST EVER!!", "2 months ago"),
	}

	got := pickTestimonials(reviews)
	require.Len(t, got, maxTestimonials)

	joined := strings.Join(got, "\n")
	for _, banned := range []string{"Terrible", "Best company", "281-555", "different company", "warranty", "fine, nothing", "⭐", "GREAT JOB"} {
		require.NotContains(t, joined, banned)
	}

	require.Contains(t, joined, "“Professional, on time and very easy to work with.” — Star E., 5★, 3 months ago")
	require.Contains(t, joined, "— Good R., 5★, 2 months ago")

	// Long reviews are cut at a sentence end.
	for _, q := range got {
		require.LessOrEqual(t, len([]rune(q)), maxTestimonialChars+60, q)
	}
}

func TestShortenEndsAtSentenceOrWord(t *testing.T) {
	t.Parallel()

	sentences := strings.Repeat("The crew was on time and tidy. ", 14)
	out := shorten(strings.TrimSpace(sentences))
	require.LessOrEqual(t, len([]rune(out)), maxTestimonialChars)
	require.True(t, strings.HasSuffix(out, "."), out)

	words := strings.Repeat("wonderful ", 60)
	out = shorten(strings.TrimSpace(words))
	require.True(t, strings.HasSuffix(out, "…"), out)
	require.LessOrEqual(t, len([]rune(out)), maxTestimonialChars+1)
}

func TestServicesFromReviewsIgnoreComplaints(t *testing.T) {
	t.Parallel()

	reviews := []review{
		rv("A", 5, "They rebuilt our fence and then the fence gate. Great roof repair too.", ""),
		rv("B", 5, "New kitchen cabinets installed on time.", ""),
		rv("C", 1, "Left my bathroom flooded and the drywall ruined.", ""),
	}

	got := servicesFromReviews(reviews)
	require.Equal(t, []string{"Fences", "Roofing", "Kitchen remodeling", "Cabinets", "Repairs and maintenance"}, got)
	require.NotContains(t, got, "Drywall", "a 1-star complaint is not evidence of a service")
	require.NotContains(t, got, "Bathroom remodeling")
}

func TestServicesForTrade(t *testing.T) {
	t.Parallel()

	require.Contains(t, servicesForTrade("Roofing contractor"), "Roof repair")
	require.Contains(t, servicesForTrade("Dry wall contractor"), "Drywall repair")
	require.Contains(t, servicesForTrade("General contractor"), "Home remodeling")
	require.Empty(t, servicesForTrade("Taco restaurant"))
}

// ---------- Build ----------

var testHeader = []string{"title", "category", "phone", "address", "open_hours", "review_rating", "review_count", "user_reviews", "website", "place_id", "link"}

func reviewsJSON(t *testing.T, reviews ...review) string {
	t.Helper()

	b, err := json.Marshal(reviews)
	require.NoError(t, err)

	return string(b)
}

func csvOf(t *testing.T, rows ...[]string) string {
	t.Helper()

	var buf bytes.Buffer

	w := csv.NewWriter(&buf)
	require.NoError(t, w.Write(testHeader))

	for _, r := range rows {
		require.NoError(t, w.Write(r))
	}

	w.Flush()
	require.NoError(t, w.Error())

	return buf.String()
}

func TestBuildStatusesAndNotes(t *testing.T) {
	t.Parallel()

	good := reviewsJSON(t, rv("Pat Lee", 5, "Wonderful roof replacement, finished a day early and spotless.", "a year ago"))
	addr := "12 Oak St, Spring, TX 77373"

	input := csvOf(t,
		[]string{"READY ROOFING LLC", "Roofing contractor", "(281) 555-0101", addr, "", "5.000000", "9", good, "", "pid1", "https://maps.example/a?authuser=0&hl=en"},
		[]string{"Low Rated Co", "General contractor", "(281) 555-0102", addr, "", "2.300000", "3", good, "", "pid2", ""},
		[]string{"Ghost Listing", "Housing development", "", addr, "", "0.000000", "0", "null", "", "pid3", ""},
		[]string{"No Reviews Yet", "General contractor", "(281) 555-0104", addr, "", "0.000000", "0", "null", "", "pid4", ""},
		[]string{"Mid Rated", "General contractor", "(281) 555-0105", addr, "", "3.700000", "3", good, "", "pid5", ""},
		[]string{"No Phone", "General contractor", "", addr, "", "5.000000", "4", good, "", "pid6", ""},
		[]string{"Facebook Only", "General contractor", "(281) 555-0107", "", "", "5.000000", "5", good, "https://www.facebook.com/x", "pid7", ""},
		[]string{"Has A Site", "General contractor", "(281) 555-0108", addr, "", "5.000000", "5", good, "https://hasasite.com", "pid8", ""},
		[]string{"Tylers's Fence", "General contractor", "(281) 555-0109", addr, "", "5.000000", "5", good, "", "pid9", ""},
	)

	briefs, err := Build(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, briefs, 9)

	by := map[string]Brief{}
	for _, b := range briefs {
		by[b.Name] = b
	}

	require.Equal(t, StatusReady, by["Ready Roofing"].Status)
	require.Equal(t, "5.0", by["Ready Roofing"].Rating)
	require.Contains(t, by["Ready Roofing"].MapsLink, "query_place_id=pid1")
	require.Equal(t, "Spring, TX and nearby areas", by["Ready Roofing"].ServiceArea)

	require.Equal(t, StatusSkip, by["Low Rated Co"].Status)
	require.Contains(t, strings.Join(by["Low Rated Co"].Notes, " "), "Reputation Management")

	require.Equal(t, StatusSkip, by["Ghost Listing"].Status, "no phone and no reviews leaves nothing to build from")
	require.Equal(t, StatusCheckFirst, by["No Reviews Yet"].Status)
	require.Equal(t, StatusCheckFirst, by["Mid Rated"].Status)
	require.Equal(t, StatusCheckFirst, by["No Phone"].Status)
	require.Equal(t, StatusCheckFirst, by["Has A Site"].Status)
	require.Equal(t, StatusCheckFirst, by["Tylers's Fence"].Status)

	// Only a social page is still a lead, and a missing street address is a note, not a blocker.
	require.Equal(t, StatusReady, by["Facebook Only"].Status)
	require.Contains(t, strings.Join(by["Facebook Only"].Notes, " "), "social or directory page")
	require.Contains(t, strings.Join(by["Facebook Only"].Notes, " "), "No street address")

	// Best candidates first.
	require.Equal(t, StatusReady, briefs[0].Status)
	require.Equal(t, StatusSkip, briefs[len(briefs)-1].Status)
}

func TestBuildRejectsOtherFilesAndAcceptsEmpty(t *testing.T) {
	t.Parallel()

	_, err := Build(strings.NewReader("Business Name,Phone\nAcme,555\n"))
	require.Error(t, err)

	briefs, err := Build(strings.NewReader(""))
	require.NoError(t, err)
	require.Empty(t, briefs)
}

func TestRowShowsExplicitNotListed(t *testing.T) {
	t.Parallel()

	briefs, err := Build(strings.NewReader(csvOf(t,
		[]string{"Bare Bones", "", "", "", "", "0.000000", "0", "null", "", "", "https://maps.example/b?x=1"},
	)))
	require.NoError(t, err)

	row := briefs[0].Row()
	require.Len(t, row, len(Headers()))

	cells := map[string]string{}
	for i, h := range Headers() {
		cells[h] = row[i]
	}

	require.Equal(t, "Not listed", cells["Phone"])
	require.Equal(t, "Not listed (service-area business)", cells["Address"])
	require.Equal(t, "No reviews yet", cells["Google Rating"])
	require.Equal(t, "0", cells["Reviews"])
	require.Equal(t, "None available (no written 4–5★ reviews)", cells["Testimonial 1"])
	require.Empty(t, cells["Testimonial 2"], "only the first cell explains that there are no quotes")
	require.Equal(t, "None mentioned in reviews", cells["Services (from reviews)"])
	require.Equal(t, "https://maps.example/b", cells["Google Maps Link"], "tracking parameters are dropped")
}

func TestPromptBranches(t *testing.T) {
	t.Parallel()

	good := reviewsJSON(t, rv("Pat Lee", 5, "Wonderful roof replacement, finished a day early and spotless.", "a year ago"))
	addr := "12 Oak St, Spring, TX 77373"

	build := func(row []string) string {
		briefs, err := Build(strings.NewReader(csvOf(t, row)))
		require.NoError(t, err)

		return briefs[0].Prompt
	}

	full := build([]string{"Acme Roofing", "Roofing contractor", "(281) 555-0101", addr, "", "5.000000", "9", good, "", "pid", ""})
	require.Contains(t, full, "- Phone: (281) 555-0101 (use tel:+12815550101 for every call button)")
	require.Contains(t, full, `"Call Now: (281) 555-0101"`)
	require.Contains(t, full, "headline for a roofing contractor serving Spring, TX")
	require.Contains(t, full, "“Wonderful roof replacement, finished a day early and spotless.” — Pat L., 5★, a year ago")
	require.Contains(t, full, "Do not invent anything")
	require.Contains(t, full, "[YOUR NAME] · [YOUR PHONE] · [YOUR EMAIL]")
	require.NotContains(t, full, "<no value>")
	require.NotContains(t, full, "%!")

	bare := build([]string{"Bare Bones", "Contractor", "", "", "", "0.000000", "0", "null", "", "", ""})
	require.Contains(t, bare, "- Phone: not listed")
	require.Contains(t, bare, `"Get Directions"`)
	require.NotContains(t, bare, `"Call Now`)
	require.Contains(t, bare, "Do NOT show a street address")
	require.Contains(t, bare, "Do NOT invent testimonials")
	require.Contains(t, bare, "Do not show a rating")
	require.Contains(t, bare, "Leave the reviews section out")
	require.Contains(t, bare, "Leave hours off the page")
}

func TestWriteCSV(t *testing.T) {
	t.Parallel()

	briefs, err := Build(strings.NewReader(csvOf(t,
		[]string{"Acme", "Roofing contractor", "(281) 555-0101", "1 A St, Spring, TX 77373", "", "5.0", "1", "null", "", "p", ""},
	)))
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, WriteCSV(&buf, briefs))
	require.True(t, strings.HasPrefix(buf.String(), "\ufeff"), "a byte order mark lets Excel read UTF-8")

	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(buf.String(), "\ufeff"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, Headers(), records[0])
	require.Len(t, records[1], len(Headers()))
	require.Contains(t, records[1][len(records[1])-1], "Acme")
}

func TestPromptNeverAsksQuestionsAndTrustsReviews(t *testing.T) {
	t.Parallel()

	review := reviewsJSON(t, rv("Sam Cruz", 5, "They installed our dishwasher and the over the range microwave quickly.", "a month ago"))
	row := []string{"Texas Home Pros", "Electrical installation service", "(281) 555-0102", "", "", "5.000000", "4", review, "", "", ""}

	briefs, err := Build(strings.NewReader(csvOf(t, row)))
	require.NoError(t, err)

	prompt := briefs[0].Prompt
	require.Contains(t, prompt, "Do not ask me any questions")
	require.Contains(t, prompt, "trust the reviews")
	require.Contains(t, prompt, "built around the work customers mention in their reviews")
	require.Contains(t, prompt, "https://www.google.com/maps/search/?api=1&query=Texas+Home+Pros", "a Maps search link is used when there is no place ID")
	require.Contains(t, prompt, `"Find us" section with a link to the Google Maps listing`)
	require.NotContains(t, prompt, "the hours and", "hours are not listed, so the prompt must not ask for them")
}

func TestMapsLinkFallback(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://maps.example/b", mapsLink("B", "", "https://maps.example/b?x=1", ""))
	require.Contains(t, mapsLink("B Co", "pid", "", ""), "query_place_id=pid")
	require.Equal(t, "https://www.google.com/maps/search/?api=1&query=B+Co+1+Main+St", mapsLink("B Co", "", "", "1 Main St"))
	require.Empty(t, mapsLink("", "", "", ""))
}

func TestLargeBusinessIsFlagged(t *testing.T) {
	t.Parallel()

	good := reviewsJSON(t, rv("Pat Lee", 5, "Wonderful work, finished early and spotless.", "a year ago"))

	statusOf := func(count string) (Status, []string) {
		briefs, err := Build(strings.NewReader(csvOf(t, []string{"Big Co", "Contractor", "(281) 555-0101", "1 Main St, Spring, TX 77373", "", "4.800000", count, good, "", "pid", ""})))
		require.NoError(t, err)

		return briefs[0].Status, briefs[0].Notes
	}

	status, notes := statusOf("2513")
	require.Equal(t, StatusCheckFirst, status)
	require.Contains(t, strings.Join(notes, "\n"), "Large business (2513 Google reviews)")

	status, notes = statusOf("40")
	require.Equal(t, StatusReady, status)
	require.NotContains(t, strings.Join(notes, "\n"), "Large business")
}
