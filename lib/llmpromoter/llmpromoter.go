package llmpromoter

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/infoe-labs/infoe-bots/lib/sqlcache"
)

const (
	// Feature name for opt-out tracking
	featureName = "llm_promoter"

	// Cooldown duration between messages to the same user (6 hours)
	cooldownDuration = 6 * time.Hour
)

var (
	// Regex to match complaints about hosted LLMs
	// This complex pattern ensures we only match actual complaints and not statements like
	// "ChatGPT is not down" or questions like "how are your Claude tokens?"
	llmComplaintRegex = regexp.MustCompile(`(?i)(?:^|\s|[.!?])(?:(?:(?:(?:my|the)\s+)?(?:claude|chatgpt|gpt-?4|windsurf|cursor|anthropic|openai|bard|gemini|llama-?2|mistral|copilot)\s+(?:is|seems|appears|has\s+been)\s+(?:(?:really|very|so|too|extremely|incredibly|unusually|currently|still|again)\s+)?(?:down|slow|unavailable|broken|unresponsive|laggy|stuck|hanging|crashing|failing|glitching|malfunctioning|problematic|acting\s+up|not\s+(?:working|responding|loading|functioning)|having\s+(?:issues|problems|difficulties|outages|downtime)))|(?:(?:can'?t|cannot|couldn'?t|unable\s+to)\s+(?:access|use|connect\s+to|log\s+into|get\s+(?:into|to\s+work)|reach)\s+(?:my|the)?\s*(?:claude|chatgpt|gpt-?4|windsurf|cursor|anthropic|openai|bard|gemini|llama-?2|mistral|copilot))|(?:(?:having|experiencing|running\s+into|facing|dealing\s+with|encountered|got)\s+(?:(?:some|major|serious|significant|constant|persistent|recurring|ongoing|frustrating)\s+)?(?:issues?|problems?|difficulties|errors?|troubles?|glitches?|bugs?|outages?|downtime|connectivity\s+problems?|performance\s+issues?|technical\s+difficulties|service\s+disruptions?)\s+(?:with|using|on|connecting\s+to)\s+(?:my|the)?\s*(?:claude|chatgpt|gpt-?4|windsurf|cursor|anthropic|openai|bard|gemini|llama-?2|mistral|copilot))|(?:(?:claude|chatgpt|gpt-?4|windsurf|cursor|anthropic|openai|bard|gemini|llama-?2|mistral|copilot)\s+(?:keeps?|constantly|repeatedly|always|still)\s+(?:crashing|failing|timing\s+out|disconnecting|giving\s+(?:me\s+)?errors?|not\s+(?:working|responding|loading)|being\s+(?:slow|unresponsive|down|unavailable)))|(?:(?:hit|reached|exceeded|maxed\s+out|used\s+up|depleted|ran\s+out\s+of|exhausted)\s+(?:my|the)\s+(?:rate\s+limits?|token\s+limits?|usage\s+limits?|quota|credits|daily\s+limits?|monthly\s+limits?|api\s+limits?)\s+(?:on|for|with)\s+(?:claude|chatgpt|gpt-?4|windsurf|cursor|anthropic|openai|bard|gemini|llama-?2|mistral|copilot))|(?:(?:claude|chatgpt|gpt-?4|windsurf|cursor|anthropic|openai|bard|gemini|llama-?2|mistral|copilot)\s+(?:outage|downtime|maintenance|service\s+disruption|technical\s+issues?|is\s+(?:down|offline|unavailable|not\s+(?:working|available|accessible|responding|loading)))))`)
)

// Initialize the SQLite cache
func init() {
	sqlcache.Initialize()
}

// HandleReactionAdd handles the reaction add event to opt out users
func HandleReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	// Ignore reactions from the bot itself
	if r.UserID == s.State.User.ID {
		return
	}

	// Check if the reaction is to one of our bot's messages
	message, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		return
	}

	// Only process if it's our bot's message and contains the opt-out text
	if message.Author.ID == s.State.User.ID && strings.Contains(strings.ToLower(message.Content), "react to this message to opt out") {

		// Add user to opt-out list
		err := sqlcache.AddOptOut(r.UserID, featureName)
		if err != nil {
			return
		}

		// Acknowledge the opt-out with a reaction and a direct message
		s.MessageReactionAdd(r.ChannelID, r.MessageID, "👍")

		// Send a DM to confirm opt-out
		channel, err := s.UserChannelCreate(r.UserID)
		if err == nil {
			s.ChannelMessageSend(channel.ID, "You've been opted out of LLM promotion messages. You won't receive these suggestions anymore.")
		}

	}
}

// HandleMessage processes messages to detect complaints about hosted LLMs
func HandleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if user has opted out
	optedOut, err := sqlcache.HasOptedOut(m.Author.ID, featureName)
	if err != nil {
		return
	}
	if optedOut {
		return
	}

	content := m.Content

	// Check if message contains complaints about hosted LLMs
	if llmComplaintRegex.MatchString(content) {
		// Check if we're in cooldown period for this user
		if !sqlcache.CanSendMessage(m.Author.ID, featureName, cooldownDuration) {
			// Still in cooldown, don't send a message
			return
		}
		// Construct the response message
		message := fmt.Sprintf("Hey <@%s>, I noticed you might be having issues with a hosted LLM service. "+
			"Have you considered trying local LLMs? Here are some great options:\n\n"+
			"• **Qwen** - Fast and efficient, works well on consumer hardware\n"+
			"• **Llama 3** - Open source and high quality for local deployment\n"+
			"• **Mistral** - Excellent performance with lower resource requirements\n"+
			"• **Phi-3** - Microsoft's compact but powerful model\n\n"+
			"Local LLMs give you more control, privacy, and aren't subject to service outages or token limits.\n\n"+
			"_(React to this message to opt out of these suggestions)_", m.Author.ID)

		// Send the message as a reply
		_, err := s.ChannelMessageSendReply(m.ChannelID, message, &discordgo.MessageReference{
			MessageID: m.ID,
			ChannelID: m.ChannelID,
			GuildID:   m.GuildID,
		})
		if err != nil {
			// Fallback to regular message if reply fails
			_, err = s.ChannelMessageSend(m.ChannelID, message)
			if err != nil {
				return
			}
		}

		// Record that we sent a message to this user
		sqlcache.RecordMessageSent(m.Author.ID, featureName)
	}
}
