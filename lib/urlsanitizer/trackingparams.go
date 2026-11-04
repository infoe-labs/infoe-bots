package urlsanitizer

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// urlRegex finds candidate http(s) links in message text. Angle brackets and
// quotes are excluded because Discord and markdown commonly wrap links, and the
// remaining trailing punctuation is removed by trimURLTail.
var urlRegex = regexp.MustCompile(`https?://[^\s<>"']+`)

// trackingParamPrefixes are query-parameter name prefixes used by analytics and
// ad platforms to attribute a visit. Everything matched here carries attribution
// only: the destination page is identical without it.
var trackingParamPrefixes = []string{
	"utm_",  // Universal Traffic Parameters, understood by nearly every analytics suite
	"mtm_",  // Matomo
	"hsa_",  // HubSpot ads
	"mc_",   // Mailchimp
	"sfmc_", // Salesforce Marketing Cloud
	"sg_",   // SendGrid
	"at_",   // Marketo
	"trk_",  // HubSpot/Act-On email tracking
	"klaviyo_",
	"vero_",
	"dcm_", // Double Click Campaign Manager
}

// trackingParams are individually named analytics parameters that keep a plain
// name instead of a vendor prefix. Names are compared in lower case.
var trackingParams = map[string]bool{
	// Google
	"gclid": true, "gclsrc": true, "gbraid": true, "wbraid": true,
	"dclid": true, "ycclid": true, "yclid": true, "_gl": true, "_ga": true,
	// Meta
	"fbclid": true, "fb_action_ids": true, "fb_action_types": true,
	"fb_source": true, "fb_ref": true, "igsh": true, "igshid": true,
	// Other ad networks
	"msclkid": true, "ttclid": true, "epst": true, "_branch_match_id": true,
	"_openstat": true, "nb_sq": true, "nb_lp": true,
	// News and syndication attribution
	"ncid": true, "smid": true, "ipuid": true, "emc": true, "te": true,
	"odid": true, "shem": true, "smtyp": true, "srcpid": true, "si": true,
	// Referrer tags appended by share buttons and in-app browsers
	"ref": true, "ref_src": true, "ref_url": true, "ref_tid": true,
	// Email platforms
	"mkt_tok": true, "vero_id": true, "ml_subscriber": true, "ml_email": true,
	"_hsenc": true, "_hsmi": true, "hsid": true, "hscampaign": true,
	"hscamp": true, "hsq": true, "hscta": true, "hsutk": true, "hstgtsm": true,
}

// utmAliases are the un-prefixed twins some senders emit next to the utm_* set
// (for example both utm_source= and source=). On their own these names are too
// generic to remove, because sites also use them for real content, so they are
// dropped only when the same URL already carries a utm_* parameter.
var utmAliases = map[string]bool{
	"source": true, "medium": true, "campaign": true, "term": true, "content": true,
}

func isUTMParam(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "utm_")
}

func isTrackingParam(name string, hasUTM bool) bool {
	lowered := strings.ToLower(name)
	if trackingParams[lowered] {
		return true
	}
	if hasUTM && utmAliases[lowered] {
		return true
	}
	for _, prefix := range trackingParamPrefixes {
		if strings.HasPrefix(lowered, prefix) {
			return true
		}
	}
	return false
}

// trimURLTail removes sentence punctuation and unbalanced closing brackets that
// a link picks up from the surrounding prose, so the reposted link stays valid.
func trimURLTail(link string) string {
	for len(link) > 0 {
		last := link[len(link)-1]
		if strings.IndexByte(".,;:!?\"'", last) != -1 {
			link = link[:len(link)-1]
			continue
		}
		if last == ')' && strings.Count(link, ")") > strings.Count(link, "(") {
			link = link[:len(link)-1]
			continue
		}
		if last == ']' && strings.Count(link, "]") > strings.Count(link, "[") {
			link = link[:len(link)-1]
			continue
		}
		break
	}
	return link
}

// strippedURL returns the link with analytics parameters removed and reports
// whether anything was removed. It fails closed: a link that cannot be parsed
// unambiguously is returned unchanged rather than reposted in a mangled form.
func strippedURL(raw string) (string, bool) {
	link := trimURLTail(raw)
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		return raw, false
	}

	candidate, err := url.Parse(link)
	if err != nil || candidate.Host == "" || candidate.RawQuery == "" {
		return raw, false
	}
	params, err := url.ParseQuery(candidate.RawQuery)
	if err != nil {
		return raw, false
	}

	hasUTM := false
	for name := range params {
		if isUTMParam(name) {
			hasUTM = true
			break
		}
	}

	kept := make(url.Values, len(params))
	for name, values := range params {
		if isTrackingParam(name, hasUTM) {
			continue
		}
		kept[name] = values
	}
	if len(kept) == len(params) {
		return raw, false
	}

	candidate.RawQuery = kept.Encode()
	return candidate.String(), true
}

// stripTrackingParams rewrites every link inside free text to its tracking-free
// form. It is used where the bot already reposts the surrounding text itself, so
// anything the link picked up from the prose (a closing bracket, a full stop) is
// put back to keep the text intact.
func stripTrackingParams(content string) string {
	return urlRegex.ReplaceAllStringFunc(content, func(match string) string {
		link := trimURLTail(match)
		stripped, changed := strippedURL(link)
		if !changed {
			return match
		}
		return stripped + match[len(link):]
	})
}

// strippedTrackingLinks returns the tracking-free form of every link in the
// message that actually changed, in order of first appearance and deduplicated.
func strippedTrackingLinks(content string) []string {
	var links []string
	seen := make(map[string]bool)
	for _, match := range urlRegex.FindAllString(content, -1) {
		stripped, changed := strippedURL(match)
		if !changed || seen[stripped] {
			continue
		}
		seen[stripped] = true
		links = append(links, stripped)
	}
	return links
}

// strippedLinksHeader names the original author so the deletion footer stays
// bound to the same user, exactly like the other repost headers.
func strippedLinksHeader(userID string) string {
	return fmt.Sprintf("🧹 <@%s>, here are the same links without tracking parameters:\n\n", userID)
}

// sendStrippedLinks posts the clean links as a new message that replies to the
// original post and pre-seeds ❌ so the author can delete it. It deliberately has
// no delete call: this mode never removes the message it is reacting to. Mentions
// are not parsed, so neither the header nor the reply pings the poster.
func sendStrippedLinks(s *discordgo.Session, m *discordgo.MessageCreate, content string) {
	text := content + deletionNotice
	message, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content:         text,
		AllowedMentions: noPings,
		Reference: &discordgo.MessageReference{
			MessageID: m.ID,
			ChannelID: m.ChannelID,
			GuildID:   m.GuildID,
		},
	})
	if err != nil {
		message, err = s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content:         text,
			AllowedMentions: noPings,
		})
	}
	if err == nil {
		s.MessageReactionAdd(m.ChannelID, message.ID, "❌")
	}
}

// HandleTrackingLinks is the no-analytics mode. Unlike the social-media rewrites
// it never deletes the original message; it only adds a follow-up message in the
// same channel carrying the same links with their tracking parameters stripped.
func HandleTrackingLinks(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m == nil || m.Author == nil || m.WebhookID != "" || m.Author.Bot ||
		s.State == nil || s.State.User == nil || m.Author.ID == s.State.User.ID {
		return
	}

	links := strippedTrackingLinks(m.Content)
	if len(links) == 0 {
		return
	}

	sendStrippedLinks(s, m, strippedLinksHeader(m.Author.ID)+strings.Join(links, "\n"))
}
