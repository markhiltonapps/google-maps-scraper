package sitebrief

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// ---------- Hours ----------

var weekdays = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

var shortDay = map[string]string{
	"Monday": "Mon", "Tuesday": "Tue", "Wednesday": "Wed", "Thursday": "Thu",
	"Friday": "Fri", "Saturday": "Sat", "Sunday": "Sun",
}

// smallHoursStart matches a range that opens between 12 and 4 AM, which for a
// local business almost always means Google has a typo (e.g. "12 AM–5 PM").
var smallHoursStart = regexp.MustCompile(`(?i)^\s*(?:12|[1-4])\s*AM\b`)

// formatHours turns Google's per-day hours JSON into "Mon–Sat 8 AM–6 PM; Sun Closed".
// suspicious is true when the hours look wrong and should be left off the page.
func formatHours(raw string) (text string, suspicious bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return "", false
	}

	var byDay map[string][]string
	if err := json.Unmarshal([]byte(raw), &byDay); err != nil || len(byDay) == 0 {
		return "", false
	}

	perDay := make([]string, len(weekdays))

	for i, day := range weekdays {
		if hours, ok := byDay[day]; ok {
			perDay[i] = tidy(strings.Join(hours, ", "))
		}
	}

	for _, h := range perDay {
		for _, r := range strings.Split(h, ", ") {
			if smallHoursStart.MatchString(r) {
				return "", true
			}
		}
	}

	var parts []string

	for i := 0; i < len(weekdays); {
		if perDay[i] == "" {
			i++

			continue
		}

		j := i
		for j+1 < len(weekdays) && perDay[j+1] == perDay[i] {
			j++
		}

		label := shortDay[weekdays[i]]
		if j > i {
			label += "–" + shortDay[weekdays[j]]
		}

		parts = append(parts, label+" "+perDay[i])
		i = j + 1
	}

	return strings.Join(parts, "; "), false
}

// ---------- Reviews ----------

type review struct {
	Name        string  `json:"Name"`
	Rating      float64 `json:"Rating"`
	Description string  `json:"Description"`
	When        string  `json:"When"`
}

const (
	maxInitialsLen       = 3
	minTestimonialRating = 4
	minTestimonialChars  = 20
	maxTestimonialChars  = 320
	maxTestimonials      = 3
)

var (
	urlOrPhone     = regexp.MustCompile(`(?i)https?://|www\.|\d{3}[-.\s)]*\d{3}[-.\s]*\d{4}`)
	businessyName  = regexp.MustCompile(`(?i)\b(llc|inc|co\.?|corp|ent\.?|enterprises?|construction|renovat\w*|travel\w*|group|company|services?|roofing|builders?)\b`)
	sentenceEnd    = regexp.MustCompile(`[.!?]["”’)]?\s`)
	leadingEditing = regexp.MustCompile(`(?i)^edited\s+`)
	nicknameQuotes = regexp.MustCompile(`["“”][^"“”]*["“”]`)
	// unsafeQuote matches reviews a demo site should not quote: ones that
	// badmouth another company, are negative in tone, or make guarantee claims
	// the business owner has not confirmed.
	unsafeQuote = regexp.MustCompile(`(?i)\b(?:different|another|other|previous)\s+(?:company|contractor|roofer|builder|guy)|terrible|horrible|awful|scam|worst|rip.?off|never again|guarantee|warrant(?:y|ies)`)
)

func parseReviews(raw string) []review {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}

	var reviews []review
	if err := json.Unmarshal([]byte(raw), &reviews); err != nil {
		return nil
	}

	return reviews
}

// reviewText cleans a review body without changing the customer's wording.
func reviewText(s string) string {
	s = tidy(s)
	s = leadingEmojis.ReplaceAllString(s, "")

	return strings.TrimSpace(s)
}

// usable reports whether a review can be quoted as a testimonial.
func usable(r review, text string) bool {
	switch {
	case r.Rating < minTestimonialRating:
		return false
	case len([]rune(text)) < minTestimonialChars:
		return false
	case urlOrPhone.MatchString(text), unsafeQuote.MatchString(text):
		return false
	case strings.Contains(text, "(Original)"), strings.Contains(text, "Translated by Google"):
		return false
	}

	return true
}

// shorten cuts long text at a sentence end (or a word) and adds an ellipsis.
func shorten(text string) string {
	if len([]rune(text)) <= maxTestimonialChars {
		return text
	}

	cut := string([]rune(text)[:maxTestimonialChars])

	if locs := sentenceEnd.FindAllStringIndex(cut+" ", -1); len(locs) > 0 {
		if end := locs[len(locs)-1][0] + 1; end >= 120 {
			return strings.TrimSpace(cut[:end])
		}
	}

	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}

	return strings.TrimRight(cut, " ,;:-") + "…"
}

// reviewerName shortens a reviewer to "First L." and falls back to a neutral
// label for handles and business-like names.
func reviewerName(raw string) string {
	const fallback = "Google reviewer"

	name := tidy(nicknameQuotes.ReplaceAllString(raw, " "))
	name = tidy(name)

	if name == "" || strings.ContainsAny(name, "0123456789@_") || businessyName.MatchString(name) {
		return fallback
	}

	tokens := strings.Fields(name)

	for i, t := range tokens {
		tokens[i] = strings.Trim(t, ",")
	}

	switch len(tokens) {
	case 1:
		return capitalizeWord(tokens[0])
	default:
		first := strings.Trim(tokens[0], ".")

		if len([]rune(first)) < 2 {
			return fallback
		}

		if isTitle(tokens[0]) && len(tokens) == 2 {
			return capitalizeWord(tokens[0]) + " " + capitalizeWord(tokens[1])
		}

		last := []rune(strings.Trim(tokens[len(tokens)-1], "."))
		if len(last) == 0 {
			return capitalizeWord(first)
		}

		if tokens[0] == tokens[len(tokens)-1] {
			return fallback
		}

		return capitalizeWord(first) + " " + string(unicode.ToUpper(last[0])) + "."
	}
}

func isTitle(s string) bool {
	switch strings.ToLower(strings.Trim(s, ".")) {
	case "mr", "mrs", "ms", "dr":
		return true
	}

	return false
}

func capitalizeWord(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}

	// Initials such as TJ or CJ stay as written.
	if isShoutingWord(s) && len(r) <= maxInitialsLen {
		return s
	}

	if isShoutingWord(s) || allLower(s) {
		return string(unicode.ToUpper(r[0])) + strings.ToLower(string(r[1:]))
	}

	return s
}

func isShoutingWord(s string) bool {
	return len(s) > 1 && s == strings.ToUpper(s) && s != strings.ToLower(s)
}

func allLower(s string) bool {
	return s == strings.ToLower(s) && s != strings.ToUpper(s)
}

// pickTestimonials returns up to three formatted quotes from 4-5 star reviews
// that have real text, longest first.
func pickTestimonials(reviews []review) (quotes []string) {
	type candidate struct {
		line  string
		score int
	}

	var candidates []candidate

	for _, r := range reviews {
		text := reviewText(r.Description)
		if !usable(r, text) {
			continue
		}

		text = shorten(text)
		when := leadingEditing.ReplaceAllString(tidy(r.When), "")

		attribution := fmt.Sprintf("%s, %d★", reviewerName(r.Name), int(r.Rating))
		if when != "" {
			attribution += ", " + when
		}

		candidates = append(candidates, candidate{
			line:  "“" + text + "” — " + attribution,
			score: quoteScore(text),
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	for i := 0; i < len(candidates) && i < maxTestimonials; i++ {
		quotes = append(quotes, candidates[i].line)
	}

	return quotes
}

const (
	idealQuoteChars = 220
	scoreSlack      = 4
	penaltyLowerCap = 60
	penaltyNoPunct  = 45
	penaltyShout    = 60
	penaltyBangs    = 50
	minShoutLetters = 4
	maxBangs        = 3
)

// quoteScore ranks how well a review reads as a testimonial: a full sentence or
// two beats a fragment, and shouting, runs of "!!" and missing punctuation lose.
func quoteScore(text string) int {
	runes := []rune(text)
	score := len(runes)

	if score > idealQuoteChars {
		score = idealQuoteChars - (score-idealQuoteChars)/scoreSlack
	}

	if unicode.IsLower(runes[0]) {
		score -= penaltyLowerCap
	}

	if !strings.ContainsRune(".!?\"”’)", runes[len(runes)-1]) {
		score -= penaltyNoPunct
	}

	if strings.Contains(text, "!!") || strings.Count(text, "!") >= maxBangs {
		score -= penaltyBangs
	}

	for _, w := range strings.Fields(text) {
		letters := strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) })
		if len([]rune(letters)) >= minShoutLetters && letters == strings.ToUpper(letters) {
			score -= penaltyShout
		}
	}

	return score
}

// ---------- Services ----------

type serviceRule struct {
	label   string
	pattern *regexp.Regexp
}

func rule(label, pattern string) serviceRule {
	return serviceRule{label: label, pattern: regexp.MustCompile(`(?i)\b(?:` + pattern + `)`)}
}

// reviewServiceRules find work customers actually mention in their reviews.
var reviewServiceRules = []serviceRule{
	rule("Roofing", `roof`),
	rule("Fences", `fenc`),
	rule("Decks", `deck`),
	rule("Kitchen remodeling", `kitchen`),
	rule("Bathroom remodeling", `bath`),
	rule("Cabinets", `cabinet`),
	rule("Flooring", `floor|laminate|tile`),
	rule("Painting", `paint`),
	rule("Drywall", `drywall|sheetrock`),
	rule("Doors and windows", `doors?\b|windows?\b`),
	rule("Stairs", `stairs?\b|staircase`),
	rule("Remodeling and renovation", `remodel|renovat`),
	rule("Home additions", `additions?\b`),
	rule("Concrete and patios", `concrete|patio|driveway|slab`),
	rule("Gutters", `gutter`),
	rule("Appliance installation", `dishwasher|microwave|appliance|washer\b|dryer\b|refrigerator|stove|oven\b|cooktop|garbage disposal`),
	rule("Electrical work", `electric|wiring|outlet|light fixture|ceiling fan|breaker`),
	rule("Plumbing", `plumb|faucet|toilet|water heater|drain`),
	rule("Air conditioning and heating", `hvac|air condition|\ba/?c\b|furnace|heating`),
	rule("Storm and water-damage repair", `storm|hail|flood|water damage|hurricane|harvey|beryl`),
	rule("Siding", `siding`),
	rule("Framing", `framing`),
	rule("Repairs and maintenance", `repair|maintenance`),
}

const maxServices = 6

// servicesFromReviews lists services customers mention, most mentioned first.
func servicesFromReviews(reviews []review) []string {
	var text strings.Builder

	// Only praised work counts: a 1-star complaint does not describe a service.
	for _, r := range reviews {
		if r.Rating >= minTestimonialRating {
			text.WriteString(reviewText(r.Description))
			text.WriteString("\n")
		}
	}

	type hit struct {
		label string
		count int
		order int
	}

	var hits []hit

	for i, sr := range reviewServiceRules {
		if n := len(sr.pattern.FindAllStringIndex(text.String(), -1)); n > 0 {
			hits = append(hits, hit{sr.label, n, i})
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].count != hits[j].count {
			return hits[i].count > hits[j].count
		}

		return hits[i].order < hits[j].order
	})

	var out []string

	for i := 0; i < len(hits) && i < maxServices; i++ {
		out = append(out, hits[i].label)
	}

	return out
}

// repairsLabel is the catch-all service shown for most trades.
const repairsLabel = "Repairs and maintenance"

type tradeServices struct {
	keywords []string
	services []string
}

// typicalServices are usual offerings for a trade, shown as suggestions that the
// owner has not confirmed. Order matters: the first matching trade wins.
var typicalServices = []tradeServices{
	{[]string{"electric"}, []string{"Electrical repairs", "Lighting installation", "Ceiling fans", "Outlets and switches", "Appliance hookups", "Troubleshooting"}},
	{[]string{"plumb"}, []string{"Leak repair", "Fixture installation", "Water heaters", "Drain cleaning", "Faucets and toilets", repairsLabel}},
	{[]string{"appliance"}, []string{"Appliance installation", "Dishwashers", "Microwaves and ranges", "Garbage disposals", repairsLabel, "Hookups"}},
	{[]string{"handyman", "handy man"}, []string{"Small repairs", "Drywall patching", "Painting touch-ups", "Fixture and appliance installation", "Door and window repair", "Odd jobs and maintenance"}},
	{[]string{"air condition", "hvac", "heating"}, []string{"Air conditioning repair", "Heating repair", "System installation", "Maintenance visits", "Thermostats", "Troubleshooting"}},
	{[]string{"fence"}, []string{"Fence installation", "Fence repair", "Gates", "Decks", "Staining and sealing", repairsLabel}},
	{[]string{"floor"}, []string{"Flooring installation", "Floor repair", "Laminate and vinyl", "Tile", "Hardwood", repairsLabel}},
	{[]string{"roof"}, []string{"Roof inspections", "Roof repair", "Roof replacement", "Storm and hail damage repair", "Leak repair", "Gutters"}},
	{[]string{"dry wall", "drywall"}, []string{"Drywall installation", "Drywall repair", "Texture and finishing", "Water-damage repair", "Patching and painting", "Ceiling repair"}},
	{[]string{"remodel", "renovat"}, []string{"Kitchen remodeling", "Bathroom remodeling", "Flooring", "Cabinets and countertops", "Interior updates", repairsLabel}},
	{[]string{"home builder", "builder"}, []string{"New home construction", "Custom homes", "Home additions", "Outdoor living spaces", "Remodeling", "Project planning"}},
	{[]string{"general contractor", "construction company", "contractor"}, []string{"Home remodeling", "Kitchen and bathroom remodels", "Roofing and repairs", "Fences and decks", "Flooring", repairsLabel}},
}

func servicesForTrade(industry string) []string {
	lower := strings.ToLower(industry)

	for _, t := range typicalServices {
		for _, k := range t.keywords {
			if strings.Contains(lower, k) {
				return t.services
			}
		}
	}

	return nil
}

// notContractor reports categories that are probably not a trade business.
func notContractor(industry string) bool {
	lower := strings.ToLower(industry)

	for _, k := range []string{"developer", "development", "real estate", "apartment", "property management"} {
		if strings.Contains(lower, k) {
			return true
		}
	}

	return false
}
