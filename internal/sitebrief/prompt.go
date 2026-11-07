package sitebrief

import (
	"fmt"
	"strings"
)

// Placeholders the person using the sheet replaces (Find & Replace works across
// the whole Prompt column) with their own contact details.
const (
	placeholderName  = "[YOUR NAME]"
	placeholderPhone = "[YOUR PHONE]"
	placeholderEmail = "[YOUR EMAIL]"
)

// cityOf returns the city from a service area like "Spring, TX and nearby areas".
func cityOf(serviceArea string) string {
	city, _, found := strings.Cut(serviceArea, " and nearby")
	if !found {
		return ""
	}

	return city
}

// withArticle puts "a" or "an" in front of a lower-case noun phrase.
func withArticle(phrase string) string {
	if phrase != "" && strings.ContainsRune("aeiou", rune(phrase[0])) {
		return "an " + phrase
	}

	return "a " + phrase
}

// buildPrompt writes the copy-and-paste prompt for an AI website builder.
// telDigits is the 10-digit phone number, or "" when there is no usable phone.
// Only facts from the brief are included, and the prompt tells the AI not to
// invent anything else.
func buildPrompt(b *Brief, telDigits string, hoursOdd bool) string {
	industry := b.Industry
	if industry == "" {
		industry = "local business"
	}

	var p strings.Builder

	line := func(format string, args ...any) {
		fmt.Fprintf(&p, format+"\n", args...)
	}

	line("Act as an expert web designer and front-end developer. Build a clean, modern, single-page demo website for a local business that currently has no website of its own.")
	line("")
	line("Use ONLY the facts below. Do not invent anything: no years in business, licenses, insurance, certifications, awards, guarantees, prices, team names, or other towns and cities.")
	line("Do not ask me any questions. If something is missing or two facts disagree, make the safest choice using only the facts below, leave that detail off the page, and put a short HTML comment at the top of the file saying what you chose.")
	line("")
	line("BUSINESS")
	line("- Name: %s", b.Name)
	line("- Industry: %s", industry)

	if telDigits != "" {
		line("- Phone: %s (use tel:+1%s for every call button)", b.Phone, telDigits)
	} else {
		line("- Phone: not listed")
	}

	if b.Address != "" {
		line("- Address: %s", b.Address)
	} else {
		line("- Address: not listed (service-area business). Do NOT show a street address.")
	}

	if b.ServiceArea != "" {
		line("- Service area: %s", b.ServiceArea)
	} else {
		line("- Service area: not listed. Do not name specific cities.")
	}

	switch {
	case b.Hours != "":
		line("- Hours: %s", b.Hours)
	case hoursOdd:
		line("- Hours: not confirmed. Leave hours off the page.")
	default:
		line("- Hours: not listed. Leave hours off the page.")
	}

	if b.Rating != "" {
		line("- Google rating: %s stars from %d Google reviews", b.Rating, b.ReviewCount)
	} else {
		line("- Google rating: none yet. Do not show a rating.")
	}

	if b.MapsLink != "" {
		line("- Google Maps listing: %s", b.MapsLink)
	}

	line("")
	line("SERVICES")

	switch {
	case len(b.ReviewServices) > 0 || len(b.TypicalServices) > 0:
		if len(b.ReviewServices) > 0 {
			line("- Work customers mention in their reviews: %s.", strings.Join(b.ReviewServices, ", "))
		}

		if len(b.TypicalServices) > 0 {
			line("- Typical for this trade (not confirmed by the owner): %s.", strings.Join(b.TypicalServices, ", "))
		}

		line("Build the services grid from these, putting the first group first. Keep the wording general and make no specific promises about any service.")

		if len(b.ReviewServices) > 0 {
			line("The Google category above can be wrong or too broad. If it does not match the work customers describe in their reviews, trust the reviews: write the headline and services around that work, and do not ask me which to use.")
		}
	default:
		line("- Services are not listed. Choose six typical services for %s, keep the wording general, and make no specific promises.", withArticle(strings.ToLower(industry)))
	}

	line("")
	line("CUSTOMER REVIEWS (real Google reviews)")

	if len(b.Testimonials) > 0 {
		for i, q := range b.Testimonials {
			line("%d. %s", i+1, q)
		}

		line("Quote these exactly as written and keep each attribution. Do not add, merge or rewrite reviews.")

		if b.Rating != "" {
			line("Also show the overall rating (%s stars, %d Google reviews).", b.Rating, b.ReviewCount)
		}
	} else {
		line("No written reviews are available. Do NOT invent testimonials.")

		if b.Rating != "" {
			line("Show only the star rating (%s stars from %d Google reviews) as social proof.", b.Rating, b.ReviewCount)
		} else {
			line("Leave the reviews section out.")
		}
	}

	line("")
	line("BUILD")
	line("- One complete, responsive index.html using Tailwind CSS from the CDN (or a React + Vite project if I am working in a repo).")
	line("- Top banner: \"This is a custom preview built for %s. Like it? Claim this website: %s · %s · %s\"", b.Name, placeholderName, placeholderPhone, placeholderEmail)

	heroFor := withArticle(strings.ToLower(industry))
	if city := cityOf(b.ServiceArea); city != "" {
		heroFor += " serving " + city
	}

	if len(b.ReviewServices) > 0 {
		heroFor += " (built around the work customers mention in their reviews)"
	}

	if telDigits != "" {
		line("- Hero: a strong, specific headline for %s, a short supporting line, and a clear call to action button \"Call Now: %s\".", heroFor, b.Phone)
	} else {
		line("- Hero: a strong, specific headline for %s, a short supporting line, and a clear call to action button \"Get Directions\" that opens the Google Maps listing (no phone number is listed, so there is no Call Now button).", heroFor)
	}

	line("- Services overview grid with a simple icon for each service.")
	line("- Customer reviews / social proof section built from the reviews above.")

	var findUs []string

	if b.Hours != "" {
		findUs = append(findUs, "the hours")
	}

	if b.MapsLink != "" {
		findUs = append(findUs, "a link to the Google Maps listing")
	}

	if len(findUs) > 0 {
		line("- A \"Find us\" section with %s (only the details given above; leave out anything not listed).", strings.Join(findUs, " and "))
	}

	line("- Footer that repeats the \"claim this website\" message with %s, %s and %s.", placeholderName, placeholderPhone, placeholderEmail)
	line("- Design: trustworthy, clean, modern and mobile-first. Sturdy typography, generous spacing, one accent color that suits %s. Avoid generic filler like \"your trusted partner\". No stock photos: use clean shapes, subtle gradients or simple icons.", withArticle(strings.ToLower(industry)))
	line("- Accessible (good contrast, labelled buttons) and fast (no heavy libraries).")
	line("")
	line("Return the complete code in a single file.")

	return strings.TrimRight(p.String(), "\n")
}
