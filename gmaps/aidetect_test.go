package gmaps_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gosom/scrapemate"
	"github.com/stretchr/testify/require"

	"github.com/gosom/google-maps-scraper/gmaps"
)

func TestDetectAITools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		html        string
		globals     []string
		wantWidgets []string
		wantAIChat  string
		wantPhone   []string
	}{
		{
			name:       "no tools",
			html:       `<html><body><h1>Joe's Plumbing</h1><a href="tel:5551234">Call us</a></body></html>`,
			wantAIChat: gmaps.AIChatNo,
		},
		{
			name:        "AI-native chatbot from script tag",
			html:        `<html><head><script src="https://www.chatbase.co/embed.min.js" defer></script></head><body></body></html>`,
			wantWidgets: []string{"Chatbase"},
			wantAIChat:  gmaps.AIChatYes,
		},
		{
			name:        "AI-capable widget from inline snippet",
			html:        `<html><body><script>(function(){var s=document.createElement('script');s.src='https://widget.intercom.io/widget/abc';})()</script></body></html>`,
			wantWidgets: []string{"Intercom"},
			wantAIChat:  gmaps.AIChatPossible,
		},
		{
			name:        "live chat only is not AI",
			html:        `<html><body><script src="https://embed.tawk.to/123/default"></script></body></html>`,
			wantWidgets: []string{"tawk.to"},
			wantAIChat:  gmaps.AIChatNo,
		},
		{
			name:        "widget found via window global",
			html:        `<html><body></body></html>`,
			globals:     []string{"tidioChatApi"},
			wantWidgets: []string{"Tidio"},
			wantAIChat:  gmaps.AIChatPossible,
		},
		{
			name:        "AI-native wins over live chat",
			html:        `<html><body><script src="https://embed.tawk.to/1"></script><script src="https://cdn.voiceflow.com/widget/bundle.mjs"></script></body></html>`,
			wantWidgets: []string{"tawk.to", "Voiceflow"},
			wantAIChat:  gmaps.AIChatYes,
		},
		{
			name: "phone signals from text and vendor link",
			html: `<html><body><p>Calls are handled 24/7 by our
				AI   Receptionist.</p><a href="https://smith.ai/?ref=acme">Powered by Smith.ai</a></body></html>`,
			wantAIChat: gmaps.AIChatNo,
			wantPhone:  []string{"link: smith.ai", "text: ai receptionist"},
		},
		{
			name:       "phrase inside script code is not visible text",
			html:       `<html><body><script>var label = "ai receptionist";</script></body></html>`,
			wantAIChat: gmaps.AIChatNo,
		},
		{
			name:       "plain link to vendor homepage is not a chat widget",
			html:       `<html><body><a href="https://www.intercom.com">We love Intercom</a></body></html>`,
			wantAIChat: gmaps.AIChatNo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := gmaps.DetectAITools([]byte(tt.html), tt.globals)

			require.Equal(t, tt.wantWidgets, got.ChatWidgets)
			require.Equal(t, tt.wantAIChat, got.AIChat)
			require.Equal(t, tt.wantPhone, got.AIPhoneSignals)
		})
	}
}

func TestEmailJobAIToolsDetectionUsesRenderedHTML(t *testing.T) {
	t.Parallel()

	entry := &gmaps.Entry{ID: "input-1", WebSite: "https://example.com"}
	job := gmaps.NewEmailJob("place-1", entry, gmaps.WithoutEmailExtraction(), gmaps.WithEmailJobAIToolsDetection())

	serverHTML := []byte(`<html><body><a href="mailto:owner@example.com">mail</a></body></html>`)
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(serverHTML))
	require.NoError(t, err)

	resp := &scrapemate.Response{
		Body:     serverHTML,
		Document: doc,
		Meta: map[string]any{
			"rendered_html": `<html><body><script src="https://code.tidio.co/abc.js"></script></body></html>`,
		},
	}

	result, next, err := job.Process(context.Background(), resp)

	require.NoError(t, err)
	require.Empty(t, next)
	require.Equal(t, entry, result)
	require.Equal(t, []string{"Tidio"}, entry.ChatWidgets)
	require.Equal(t, gmaps.AIChatPossible, entry.AIChat)
	require.Empty(t, entry.Emails, "email extraction was disabled")
}

func TestEmailJobAIToolsDetectionFetchError(t *testing.T) {
	t.Parallel()

	entry := &gmaps.Entry{ID: "input-1", WebSite: "https://example.com"}
	job := gmaps.NewEmailJob("place-1", entry, gmaps.WithEmailJobAIToolsDetection())

	_, _, err := job.Process(context.Background(), &scrapemate.Response{Error: errors.New("fetch failed")})

	require.NoError(t, err)
	require.Equal(t, gmaps.AIChatUnknown, entry.AIChat)
}

func TestEntryCsvIncludesAIToolsColumns(t *testing.T) {
	t.Parallel()

	entry := gmaps.Entry{
		ChatWidgets:    []string{"Intercom", "tawk.to"},
		AIChat:         gmaps.AIChatPossible,
		AIPhoneSignals: []string{"text: ai receptionist"},
	}

	headers := entry.CsvHeaders()
	row := entry.CsvRow()

	require.Len(t, row, len(headers))
	require.Equal(t, []string{"chat_widgets", "ai_chat", "ai_phone_signals"}, headers[len(headers)-3:])
	require.Equal(t, []string{"Intercom, tawk.to", "possible", "text: ai receptionist"}, row[len(row)-3:])

	// Without detection the original columns are kept, so CSV files written
	// before this feature existed can still be resumed.
	plain := gmaps.Entry{}
	require.Equal(t, "emails", plain.CsvHeaders()[len(plain.CsvHeaders())-1])
	require.Len(t, plain.CsvRow(), len(plain.CsvHeaders()))
	require.Len(t, headers, len(plain.CsvHeaders())+3)
}

func TestMarkAIToolsSkipped(t *testing.T) {
	t.Parallel()

	noSite := gmaps.Entry{}
	noSite.MarkAIToolsSkipped()
	require.Equal(t, gmaps.AIChatNoWebsite, noSite.AIChat)

	social := gmaps.Entry{WebSite: "https://facebook.com/acme"}
	social.MarkAIToolsSkipped()
	require.Equal(t, gmaps.AIChatNotChecked, social.AIChat)
}
