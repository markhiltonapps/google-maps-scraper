// Package sitebrief turns a scraper results CSV into a clean "website brief"
// spreadsheet: one row per business with tidy contact details, hours, real
// customer testimonials and a ready-to-paste prompt for an AI website builder.
//
// Scraped data is messy (JSON blobs, ALL CAPS names, odd hours, negative and
// empty reviews), so everything that could end up on a customer-facing page is
// normalized here, and anything doubtful is flagged instead of guessed.
package sitebrief

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	spaceRun       = regexp.MustCompile(`\s+`)
	companySuffix  = regexp.MustCompile(`(?i)[,\s]+(l\.?l\.?c\.?|inc\.?|incorporated)\s*$`)
	doublePossess  = regexp.MustCompile(`(?i)[a-z]s's\b`)
	leadingEmojis  = regexp.MustCompile(`^[\s\x{2B50}\x{2605}\x{2606}\x{FE0F}\x{200D}]+`)
	invisibleRunes = strings.NewReplacer(
		"\u202f", " ", // narrow no-break space, used by Google between "8" and "AM"
		"\u00a0", " ",
		"\u200b", "",
		"\u200c", "",
		"\u200d", "",
		"\ufeff", "",
	)
)

// acronyms stay upper case when a shouting name is converted to title case.
var acronyms = map[string]bool{
	"LLC": true, "INC": true, "LP": true, "LLP": true, "USA": true, "HVAC": true,
	"TX": true, "II": true, "III": true, "AC": true, "HOA": true,
}

// tidy removes invisible characters and collapses whitespace.
func tidy(s string) string {
	return strings.TrimSpace(spaceRun.ReplaceAllString(invisibleRunes.Replace(s), " "))
}

// cleanName tidies a business name for display: ALL CAPS becomes title case and
// a trailing "LLC"/"Inc" is dropped. Mixed-case names are left alone.
func cleanName(raw string) string {
	name := tidy(raw)

	if isShouting(name) {
		name = titleCase(name)
	}

	name = companySuffix.ReplaceAllString(name, "")

	return strings.TrimRight(strings.TrimSpace(name), ",")
}

// isShouting reports whether every letter is upper case and there are enough
// letters for that to be a style rather than an acronym.
func isShouting(s string) bool {
	letters := 0

	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++

			if unicode.IsLower(r) {
				return false
			}
		}
	}

	return letters > 4
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = titleWord(w)
	}

	return strings.Join(words, " ")
}

func titleWord(w string) string {
	core := strings.Trim(w, ",.()")
	if acronyms[strings.ToUpper(core)] {
		return strings.ToUpper(w)
	}

	letters := 0

	for _, r := range core {
		if unicode.IsLetter(r) {
			letters++
		}
	}

	// Short all-caps words without vowels are initials (CJC, RSL): keep them.
	if letters <= 3 && !strings.ContainsAny(strings.ToLower(core), "aeiou") {
		return w
	}

	runes := []rune(strings.ToLower(w))
	upperNext := true

	for i, r := range runes {
		if unicode.IsLetter(r) && upperNext {
			runes[i] = unicode.ToUpper(r)
		}

		// Capitalize after hyphens and after a one-letter prefix like O'Brien.
		upperNext = r == '-' || (r == '\'' && i == 1)
	}

	return string(runes)
}

// nameLooksOff flags a name with "s's" in it, for the Notes column. That is
// often a typo ("Tylers's" for "Tyler's") but can be right ("James's"), so it
// only asks for a second look.
func nameLooksOff(name string) string {
	if doublePossess.MatchString(name) {
		return "Name has an \"s's\" ending, which is sometimes a typo (Tylers's vs Tyler's): check the spelling."
	}

	return ""
}

// formatPhone returns a US phone number as (281) 555-1234. ok is false when the
// number is blank or not a recognizable 10-digit number.
func formatPhone(raw string) (formatted, digits string, ok bool) {
	var b strings.Builder

	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}

	d := b.String()
	if len(d) == 11 && d[0] == '1' {
		d = d[1:]
	}

	if len(d) != 10 {
		return tidy(raw), "", false
	}

	return "(" + d[:3] + ") " + d[3:6] + "-" + d[6:], d, true
}
