package gmaps

import (
	"bytes"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Values for Entry.AIChat.
const (
	// AIChatYes means a chatbot built specifically for AI conversations was found.
	AIChatYes = "yes"
	// AIChatPossible means a chat tool that offers an optional AI agent was found.
	AIChatPossible = "possible"
	// AIChatNo means the website loaded and no AI-capable chat tool was found.
	AIChatNo = "no"
	// AIChatUnknown means the website could not be loaded.
	AIChatUnknown = "unknown"
	// AIChatNoWebsite means the place has no website listed on Google Maps.
	AIChatNoWebsite = "no website"
	// AIChatNotChecked means the website was skipped (e.g. a social media page).
	AIChatNotChecked = "not checked"
)

type chatVendorKind int

const (
	// chatVendorLiveChat is staffed by humans; any AI features are minor.
	chatVendorLiveChat chatVendorKind = iota
	// chatVendorAICapable offers an AI agent the business may or may not have enabled.
	chatVendorAICapable
	// chatVendorAINative exists to run AI chatbots.
	chatVendorAINative
)

type chatVendor struct {
	name string
	kind chatVendorKind
	// patterns are matched against script/iframe URLs and inline script code.
	patterns []string
	// globals are JavaScript globals the vendor's widget defines on window.
	globals []string
}

var chatVendors = []chatVendor{
	{name: "Intercom", kind: chatVendorAICapable, patterns: []string{"widget.intercom.io", "js.intercomcdn.com"}, globals: []string{"Intercom"}},
	{name: "Drift", kind: chatVendorAICapable, patterns: []string{"js.driftt.com"}, globals: []string{"drift"}},
	{name: "Tidio", kind: chatVendorAICapable, patterns: []string{"code.tidio.co"}, globals: []string{"tidioChatApi"}},
	{name: "Zendesk", kind: chatVendorAICapable, patterns: []string{"static.zdassets.com/ekr/snippet.js", "ekr.zdassets.com"}, globals: []string{"zE"}},
	{name: "HubSpot Chat", kind: chatVendorAICapable, patterns: []string{"js.usemessages.com"}, globals: []string{"HubSpotConversations"}},
	{name: "Freshchat", kind: chatVendorAICapable, patterns: []string{"wchat.freshchat.com"}, globals: []string{"fcWidget"}},
	{name: "Crisp", kind: chatVendorAICapable, patterns: []string{"client.crisp.chat"}, globals: []string{"$crisp"}},
	{name: "Gorgias", kind: chatVendorAICapable, patterns: []string{"config.gorgias.chat"}, globals: []string{"GorgiasChat"}},
	{name: "Podium", kind: chatVendorAICapable, patterns: []string{"connect.podium.com"}},
	{name: "LeadConnector (GoHighLevel)", kind: chatVendorAICapable, patterns: []string{"widgets.leadconnectorhq.com"}},
	{name: "Landbot", kind: chatVendorAICapable, patterns: []string{"static.landbot.io", "chats.landbot.io"}, globals: []string{"Landbot"}},
	{name: "LiveChat", kind: chatVendorLiveChat, patterns: []string{"cdn.livechatinc.com"}, globals: []string{"LiveChatWidget"}},
	{name: "tawk.to", kind: chatVendorLiveChat, patterns: []string{"embed.tawk.to"}, globals: []string{"Tawk_API"}},
	{name: "Olark", kind: chatVendorLiveChat, patterns: []string{"static.olark.com"}, globals: []string{"olark"}},
	{name: "Smartsupp", kind: chatVendorLiveChat, patterns: []string{"smartsuppchat.com"}, globals: []string{"smartsupp"}},
	{name: "Ada", kind: chatVendorAINative, patterns: []string{"static.ada.support"}, globals: []string{"adaEmbed"}},
	{name: "Chatbase", kind: chatVendorAINative, patterns: []string{"chatbase.co/embed"}, globals: []string{"chatbase"}},
	{name: "Voiceflow", kind: chatVendorAINative, patterns: []string{"cdn.voiceflow.com"}, globals: []string{"voiceflow"}},
	{name: "Botpress", kind: chatVendorAINative, patterns: []string{"cdn.botpress.cloud"}, globals: []string{"botpress", "botpressWebChat"}},
}

// aiPhoneVendorDomains are AI receptionist / voice agent providers. A link,
// script or iframe pointing at one of them suggests the business uses it.
var aiPhoneVendorDomains = []string{
	"smith.ai",
	"goodcall.com",
	"dialzara.com",
	"myaifrontdesk.com",
	"retellai.com",
	"synthflow.ai",
	"vapi.ai",
	"bland.ai",
}

// aiPhonePhrases are matched against the visible text of the page.
var aiPhonePhrases = []string{
	"ai receptionist",
	"ai-powered receptionist",
	"ai phone agent",
	"ai phone assistant",
	"ai answering service",
	"ai voice agent",
	"ai voice assistant",
	"answered by ai",
	"virtual receptionist",
}

// AIToolsResult holds the chat and phone signals found on a website.
type AIToolsResult struct {
	ChatWidgets    []string
	AIChat         string
	AIPhoneSignals []string
}

// ChatWidgetGlobals returns the window globals that identify chat widgets,
// for checking in a live browser page.
func ChatWidgetGlobals() []string {
	var globals []string

	for i := range chatVendors {
		globals = append(globals, chatVendors[i].globals...)
	}

	return globals
}

// DetectAITools inspects a website's HTML (ideally rendered after JavaScript
// ran) and the window globals present on the page for AI chat and AI phone
// agent signals.
func DetectAITools(html []byte, globals []string) AIToolsResult {
	result := AIToolsResult{AIChat: AIChatNo}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return result
	}

	resources := collectResources(doc)
	links := collectLinks(doc)
	text := visibleText(doc)

	presentGlobals := make(map[string]bool, len(globals))
	for _, g := range globals {
		presentGlobals[g] = true
	}

	best := -1

	for i := range chatVendors {
		vendor := &chatVendors[i]
		if !vendorPresent(vendor, resources, presentGlobals) {
			continue
		}

		result.ChatWidgets = append(result.ChatWidgets, vendor.name)

		if int(vendor.kind) > best {
			best = int(vendor.kind)
		}
	}

	switch chatVendorKind(best) {
	case chatVendorAINative:
		result.AIChat = AIChatYes
	case chatVendorAICapable:
		result.AIChat = AIChatPossible
	case chatVendorLiveChat:
		result.AIChat = AIChatNo
	}

	for _, domain := range aiPhoneVendorDomains {
		if strings.Contains(resources, domain) || strings.Contains(links, domain) {
			result.AIPhoneSignals = append(result.AIPhoneSignals, "link: "+domain)
		}
	}

	for _, phrase := range aiPhonePhrases {
		if strings.Contains(text, phrase) {
			result.AIPhoneSignals = append(result.AIPhoneSignals, "text: "+phrase)
		}
	}

	return result
}

func vendorPresent(vendor *chatVendor, resources string, globals map[string]bool) bool {
	for _, g := range vendor.globals {
		if globals[g] {
			return true
		}
	}

	for _, p := range vendor.patterns {
		if strings.Contains(resources, p) {
			return true
		}
	}

	return false
}

// collectResources returns script/iframe URLs and inline script code, lowercased.
func collectResources(doc *goquery.Document) string {
	var sb strings.Builder

	doc.Find("script, iframe, link").Each(func(_ int, s *goquery.Selection) {
		for _, attr := range []string{"src", "href", "data-src"} {
			if v, ok := s.Attr(attr); ok {
				sb.WriteString(v)
				sb.WriteByte('\n')
			}
		}

		if goquery.NodeName(s) == "script" {
			sb.WriteString(s.Text())
			sb.WriteByte('\n')
		}
	})

	return strings.ToLower(sb.String())
}

// collectLinks returns anchor hrefs, lowercased.
func collectLinks(doc *goquery.Document) string {
	var sb strings.Builder

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		sb.WriteString(s.AttrOr("href", ""))
		sb.WriteByte('\n')
	})

	return strings.ToLower(sb.String())
}

// visibleText returns the page text without script/style content, lowercased
// and with whitespace collapsed so phrases split across lines still match.
func visibleText(doc *goquery.Document) string {
	body := doc.Find("body").Clone()
	body.Find("script, style, noscript, template").Remove()

	return strings.ToLower(strings.Join(strings.Fields(body.Text()), " "))
}
