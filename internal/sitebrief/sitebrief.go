package sitebrief

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gosom/google-maps-scraper/gmaps"
)

// Status says whether a business is a good candidate for a demo website.
type Status string

// The three statuses, best first.
const (
	StatusReady      Status = "Ready"
	StatusCheckFirst Status = "Check first"
	StatusSkip       Status = "Skip"
)

const (
	skipRatingAtOrBelow = 3.0
	minGoodRating       = 4.0
	minReviewsForSkip   = 2
	thinReviewCount     = 2
	// largeBusinessReviews is the review count above which a business with no
	// website listed is probably big enough to have one elsewhere.
	largeBusinessReviews = 200
)

var addressCityState = regexp.MustCompile(`,\s*([^,]+),\s*([A-Z]{2})\s+\d{5}`)

// Brief is one cleaned row of the website brief spreadsheet.
type Brief struct {
	Status       Status
	Notes        []string
	Name         string
	Industry     string
	Phone        string
	Address      string
	ServiceArea  string
	Hours        string
	HoursUnclear bool
	Rating       string
	ReviewCount  int
	Testimonials []string
	// ReviewServices are services customers mention; TypicalServices are
	// suggestions for the trade that the owner has not confirmed.
	ReviewServices  []string
	TypicalServices []string
	MapsLink        string
	Prompt          string
}

// Headers are the spreadsheet column titles, in order.
func Headers() []string {
	return []string{
		"Status", "Notes", "Business Name", "Industry", "Phone", "Address", "Service Area", "Hours",
		"Google Rating", "Reviews", "Testimonial 1", "Testimonial 2", "Testimonial 3",
		"Services (from reviews)", "Services (typical for this trade)", "Google Maps Link",
		"Website Prompt (copy & paste)",
	}
}

// Row returns the brief as spreadsheet cells matching Headers.
func (b *Brief) Row() []string {
	quotes := make([]string, maxTestimonials)
	copy(quotes, b.Testimonials)

	if len(b.Testimonials) == 0 {
		quotes[0] = "None available (no written 4–5★ reviews)"
	}

	rating := b.Rating
	if rating == "" {
		rating = "No reviews yet"
	}

	address := b.Address
	if address == "" {
		address = "Not listed (service-area business)"
	}

	hours := orNotListed(b.Hours)
	if b.HoursUnclear {
		hours = "Needs checking (looks wrong on Google)"
	}

	return []string{
		string(b.Status), "• " + strings.Join(b.Notes, "\n• "), b.Name, orNotListed(b.Industry), orNotListed(b.Phone), address,
		orNotListed(b.ServiceArea), hours, rating, strconv.Itoa(b.ReviewCount), quotes[0], quotes[1], quotes[2],
		orDefault(strings.Join(b.ReviewServices, ", "), "None mentioned in reviews"),
		orDefault(strings.Join(b.TypicalServices, ", "), "Not listed"), b.MapsLink, b.Prompt,
	}
}

// mapsLink builds a short, stable Google Maps link from the place ID, falling
// back to the scraped link without its tracking parameters.
func mapsLink(name, placeID, scraped, address string) string {
	if placeID = strings.TrimSpace(placeID); placeID != "" {
		return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(name) + "&query_place_id=" + url.QueryEscape(placeID)
	}

	if link, _, _ := strings.Cut(strings.TrimSpace(scraped), "?"); link != "" {
		return link
	}

	// No place ID or link in the data: fall back to a Maps search by name and
	// street address, which still opens the right business in most cases.
	if name = strings.TrimSpace(name); name == "" {
		return ""
	}

	query := name
	if address != "" {
		query += " " + address
	}

	return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(query)
}

// orNotListed shows an explicit "Not listed" rather than an empty cell.
func orNotListed(s string) string {
	return orDefault(s, "Not listed")
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}

	return s
}

// Build reads a full scraper results CSV (the "Full data" download) and returns
// one brief per business, best candidates first.
func Build(src io.Reader) ([]Brief, error) {
	briefs, err := BuildInOrder(src)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(briefs, func(i, j int) bool {
		if rank(briefs[i].Status) != rank(briefs[j].Status) {
			return rank(briefs[i].Status) < rank(briefs[j].Status)
		}

		return briefs[i].ReviewCount > briefs[j].ReviewCount
	})

	return briefs, nil
}

// BuildInOrder is like Build but keeps the briefs in the same order as the rows
// of the results CSV, so each brief lines up with its source row.
func BuildInOrder(src io.Reader) ([]Brief, error) {
	reader := csv.NewReader(src)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	index := make(map[string]int, len(header))

	for i, h := range header {
		index[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}

	if _, ok := index["title"]; !ok {
		return nil, errors.New("not a scraper results file: missing the \"title\" column")
	}

	var briefs []Brief

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}

		field := func(name string) string {
			if i, ok := index[name]; ok && i < len(record) {
				return record[i]
			}

			return ""
		}

		briefs = append(briefs, fromRecord(field))
	}

	return briefs, nil
}

func rank(s Status) int {
	switch s {
	case StatusReady:
		return 0
	case StatusCheckFirst:
		return 1
	case StatusSkip:
		return 2
	default:
		return 2
	}
}

func fromRecord(field func(string) string) Brief {
	rawName := field("title")
	b := Brief{
		Name:     cleanName(rawName),
		Industry: tidy(field("category")),
		Address:  tidy(field("address")),
	}

	b.MapsLink = mapsLink(b.Name, field("place_id"), field("link"), b.Address)

	var notes []string

	note := func(s string) { notes = append(notes, s) }

	check := false // becomes true when a person should look before building

	// Phone.
	phone, digits, phoneOK := formatPhone(field("phone"))
	b.Phone = phone
	hasPhone := phoneOK

	if !phoneOK {
		if b.Phone == "" {
			note("No phone number listed: the page can't have a Call Now button, so it links to Google Maps instead.")
		} else {
			note("Phone number looks unusual (" + b.Phone + "): check it.")
		}

		check = true
	}

	// Address and service area.
	if m := addressCityState.FindStringSubmatch(b.Address); m != nil {
		b.ServiceArea = m[1] + ", " + m[2] + " and nearby areas"
	}

	if b.Address == "" {
		note("No street address (a service-area business): the page leaves the address out. Its map pin is the middle of its service area, so check it is in your target area.")
	}

	// Hours.
	hours, odd := formatHours(field("open_hours"))
	b.Hours = hours
	b.HoursUnclear = odd

	if odd {
		note("Opening hours look wrong on Google (for example a day that opens at 12 AM): they are left off the page. Confirm with the owner.")

		check = true
	}

	// Reviews.
	reviews := parseReviews(field("user_reviews"))
	b.Testimonials = pickTestimonials(reviews)
	b.ReviewServices = servicesFromReviews(reviews)
	b.TypicalServices = servicesForTrade(b.Industry)

	rating, _ := strconv.ParseFloat(strings.TrimSpace(field("review_rating")), 64)
	count, _ := strconv.Atoi(strings.TrimSpace(field("review_count")))
	b.ReviewCount = count

	if count > 0 && rating > 0 {
		b.Rating = strconv.FormatFloat(rating, 'f', 1, 64)
	}

	skip := false

	switch {
	case count == 0:
		note("No Google reviews yet: there is nothing to show as social proof.")

		check = true

		if !hasPhone {
			note("No phone and no reviews: there is almost nothing to build a page from.")

			skip = true
		}
	case rating > 0 && rating <= skipRatingAtOrBelow && count >= minReviewsForSkip:
		note(fmt.Sprintf("Rated %.1f★ from %d reviews: a poor fit for a showcase site. Better prospect for Reputation Management.", rating, count))

		skip = true
	case rating > 0 && rating < minGoodRating:
		note(fmt.Sprintf("Rated %.1f★: below 4 stars, so check whether you want to feature this business.", rating))

		check = true
	}

	if count > 0 && len(b.Testimonials) == 0 {
		note("No usable written reviews (only star ratings, or no 4-5★ reviews with text): the page shows the star rating only.")

		check = true
	}

	if count > 0 && count <= thinReviewCount && len(b.Testimonials) > 0 {
		note(fmt.Sprintf("Only %d review: thin social proof.", count))
	}

	if count >= largeBusinessReviews {
		note(fmt.Sprintf("Large business (%d Google reviews): it probably has a website that Google doesn't list. Search its name on Google before building.", count))

		check = true
	}

	// Name, category and website checks.
	if s := nameLooksOff(rawName); s != "" {
		note(s)

		check = true
	}

	if notContractor(b.Industry) {
		note("Category is \"" + b.Industry + "\": may not be a contractor or service business.")

		check = true
	}

	if site := strings.TrimSpace(field("website")); site != "" {
		if gmaps.IsOwnWebsiteURL(site) {
			note("Already lists a website (" + site + "): this is not a no-website lead.")

			check = true
		} else {
			note("Only links a social or directory page, not a website of its own.")
		}
	}

	switch {
	case skip:
		b.Status = StatusSkip
	case check:
		b.Status = StatusCheckFirst
	default:
		b.Status = StatusReady
	}

	if len(notes) == 0 {
		note("Complete: phone, reviews and details all look good.")
	}

	b.Notes = notes
	b.Prompt = buildPrompt(&b, digits, odd)

	return b
}

// WriteCSV writes briefs as a UTF-8 CSV that opens correctly in Excel (with a
// byte order mark so quotes and stars display properly).
func WriteCSV(w io.Writer, briefs []Brief) error {
	if _, err := io.WriteString(w, "\ufeff"); err != nil {
		return err
	}

	writer := csv.NewWriter(w)

	if err := writer.Write(Headers()); err != nil {
		return err
	}

	for i := range briefs {
		if err := writer.Write(briefs[i].Row()); err != nil {
			return err
		}
	}

	writer.Flush()

	return writer.Error()
}
