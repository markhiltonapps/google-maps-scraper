package gmaps

import (
	"context"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/PuerkitoBio/goquery"
	"github.com/gosom/scrapemate"
	"github.com/mcnijman/go-emailaddress"

	"github.com/gosom/google-maps-scraper/exiter"
)

const (
	renderedHTMLMetaKey = "rendered_html"
	jsGlobalsMetaKey    = "js_globals"

	// chatWidgetSettleTime gives chat widgets, which are usually injected by
	// JavaScript after the page has loaded, time to appear in the DOM.
	chatWidgetSettleTime = 4 * time.Second

	// presentGlobalsJS returns which of the given names are defined on window.
	presentGlobalsJS = `(names) => names.filter((n) => {
		try { return typeof window[n] !== 'undefined'; } catch (e) { return false; }
	})`
)

type EmailExtractJobOptions func(*EmailExtractJob)

// EmailExtractJob visits a place's website to extract emails and, optionally,
// detect AI chat and AI phone agent tools.
type EmailExtractJob struct {
	scrapemate.Job

	Entry                   *Entry
	ExitMonitor             exiter.Exiter
	WriterManagedCompletion bool
	ExtractEmail            bool
	DetectAITools           bool
}

func NewEmailJob(parentID string, entry *Entry, opts ...EmailExtractJobOptions) *EmailExtractJob {
	const (
		defaultPrio       = scrapemate.PriorityHigh
		defaultMaxRetries = 0
	)

	job := EmailExtractJob{
		Job: scrapemate.Job{
			ID:         uuid.NewV4().String(),
			ParentID:   parentID,
			Method:     requestMethodGet,
			URL:        normalizeGoogleURL(entry.WebSite),
			MaxRetries: defaultMaxRetries,
			Priority:   defaultPrio,
		},
	}

	job.Entry = entry
	job.ExtractEmail = true

	for _, opt := range opts {
		opt(&job)
	}

	return &job
}

func WithEmailJobExitMonitor(exitMonitor exiter.Exiter) EmailExtractJobOptions {
	return func(j *EmailExtractJob) {
		j.ExitMonitor = exitMonitor
	}
}

func WithEmailJobWriterManagedCompletion() EmailExtractJobOptions {
	return func(j *EmailExtractJob) {
		j.WriterManagedCompletion = true
	}
}

// WithEmailJobAIToolsDetection makes the job check the website for AI chat
// and AI phone agent tools.
func WithEmailJobAIToolsDetection() EmailExtractJobOptions {
	return func(j *EmailExtractJob) {
		j.DetectAITools = true
	}
}

// WithoutEmailExtraction makes the job skip email extraction, for when the
// website is only visited to detect AI tools.
func WithoutEmailExtraction() EmailExtractJobOptions {
	return func(j *EmailExtractJob) {
		j.ExtractEmail = false
	}
}

// BrowserActions loads the website. When AI tools detection is enabled it
// also captures the rendered HTML and the chat widget globals present on the
// page, since most chat widgets are injected by JavaScript and are missing
// from the HTML the server sends.
func (j *EmailExtractJob) BrowserActions(ctx context.Context, page scrapemate.BrowserPage) scrapemate.Response {
	if !j.DetectAITools {
		return j.Job.BrowserActions(ctx, page)
	}

	var resp scrapemate.Response

	// Wait for "load" plus a fixed settle time instead of network idle: chat
	// widgets often keep connections open, so pages that have one may never
	// become idle and would time out.
	pageResponse, err := page.Goto(j.GetFullURL(), scrapemate.WaitUntilLoad)
	if err != nil {
		resp.Error = err

		return resp
	}

	resp.URL = pageResponse.URL
	resp.StatusCode = pageResponse.StatusCode
	resp.Headers = pageResponse.Headers
	resp.Body = pageResponse.Body

	page.WaitForTimeout(chatWidgetSettleTime)

	resp.Meta = map[string]any{}

	if html, err := page.Content(); err == nil {
		resp.Meta[renderedHTMLMetaKey] = html
	}

	if raw, err := page.Eval(presentGlobalsJS, ChatWidgetGlobals()); err == nil {
		if names, ok := raw.([]any); ok {
			resp.Meta[jsGlobalsMetaKey] = toStringSlice(names)
		}
	}

	return resp
}

func (j *EmailExtractJob) Process(ctx context.Context, resp *scrapemate.Response) (any, []scrapemate.IJob, error) {
	defer func() {
		resp.Document = nil
		resp.Body = nil
		resp.Meta = nil
	}()

	defer func() {
		if j.ExitMonitor != nil && !j.WriterManagedCompletion {
			j.ExitMonitor.IncrPlacesCompleted(1)
		}
	}()

	log := scrapemate.GetLoggerFromContext(ctx)

	log.Info("Processing email job", "url", j.URL)

	if j.DetectAITools {
		if resp.Error != nil {
			log.Warn("Website fetch failed, AI tools unknown", "url", j.URL, "error", resp.Error)
		}

		j.detectAITools(resp)
	}

	if !j.ExtractEmail {
		return j.Entry, nil, nil
	}

	// if html fetch failed just return
	if resp.Error != nil {
		return j.Entry, nil, nil
	}

	doc, ok := resp.Document.(*goquery.Document)
	if !ok {
		return j.Entry, nil, nil
	}

	emails := CleanEmails(docEmailExtractor(doc))
	if len(emails) == 0 {
		emails = CleanEmails(regexEmailExtractor(resp.Body))
	}

	j.Entry.Emails = emails

	return j.Entry, nil, nil
}

func (j *EmailExtractJob) ProcessOnFetchError() bool {
	return true
}

func (j *EmailExtractJob) detectAITools(resp *scrapemate.Response) {
	if resp.Error != nil {
		j.Entry.AIChat = AIChatUnknown

		return
	}

	// Fall back to the server HTML when the page was not fetched with a
	// browser (e.g. the HTTP fetcher), so script-tag widgets are still found.
	html := resp.Body
	if rendered, ok := resp.Meta[renderedHTMLMetaKey].(string); ok && rendered != "" {
		html = []byte(rendered)
	}

	globals, _ := resp.Meta[jsGlobalsMetaKey].([]string)

	j.Entry.SetAIToolsResult(DetectAITools(html, globals))
}

// nonEmailSuffixes are file extensions that look like an email domain's
// ending in asset names such as "logo@2x.png".
var nonEmailSuffixes = []string{
	".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico", ".avif", ".tif", ".tiff",
	".js", ".css", ".mp4", ".webm", ".pdf", ".woff", ".woff2", ".ttf",
}

// CleanEmails drops strings that are file names rather than email addresses
// (e.g. "logo-300x149@2x.png") and removes case-insensitive duplicates,
// keeping the first spelling seen.
func CleanEmails(emails []string) []string {
	seen := make(map[string]bool, len(emails))
	out := make([]string, 0, len(emails))

	for _, email := range emails {
		email = strings.TrimSpace(email)
		lower := strings.ToLower(email)

		if email == "" || seen[lower] || hasNonEmailSuffix(lower) {
			continue
		}

		seen[lower] = true

		out = append(out, email)
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func hasNonEmailSuffix(lower string) bool {
	for _, suffix := range nonEmailSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}

	return false
}

func docEmailExtractor(doc *goquery.Document) []string {
	seen := map[string]bool{}

	var emails []string

	doc.Find("a[href^='mailto:']").Each(func(_ int, s *goquery.Selection) {
		mailto, exists := s.Attr("href")
		if exists {
			value := strings.TrimPrefix(mailto, "mailto:")
			if email, err := getValidEmail(value); err == nil {
				if !seen[email] {
					emails = append(emails, email)
					seen[email] = true
				}
			}
		}
	})

	return emails
}

func regexEmailExtractor(body []byte) []string {
	seen := map[string]bool{}

	var emails []string

	addresses := emailaddress.Find(body, false)
	for i := range addresses {
		if !seen[addresses[i].String()] {
			emails = append(emails, addresses[i].String())
			seen[addresses[i].String()] = true
		}
	}

	return emails
}

func getValidEmail(s string) (string, error) {
	email, err := emailaddress.Parse(strings.TrimSpace(s))
	if err != nil {
		return "", err
	}

	return email.String(), nil
}

// normalizeGoogleURL extracts the actual target URL from Google redirect URLs.
// Google Maps sometimes returns URLs like "/url?q=http://example.com/&opi=..."
// for external website links.
func normalizeGoogleURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	if strings.HasPrefix(rawURL, "/url?q=") {
		fullURL := "https://www.google.com" + rawURL

		parsed, err := url.Parse(fullURL)
		if err != nil {
			return rawURL
		}

		if target := parsed.Query().Get("q"); target != "" {
			return target
		}
	}

	if strings.HasPrefix(rawURL, "/") {
		return "https://www.google.com" + rawURL
	}

	return rawURL
}
