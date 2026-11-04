package urlsanitizer

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var (
	instagramReelRegex = regexp.MustCompile(`https://(?:www\.)?instagram\.com/reels?/[^\s?]+(?:\?[^\s]*)?`)
	facebookShareRegex = regexp.MustCompile(`https://(?:www\.|m\.)?facebook\.com/share/[^\s]+`)
	tiktokRegex        = regexp.MustCompile(`https://(?:www\.)?tiktok\.com/(?:@[^/]+/?|t/[^/]+/?)$`)
	// Match both x.com and twitter.com URLs
	twitterRegex = regexp.MustCompile(`https://(?:(?:www\.)?(?:twitter|x)\.com/[^/]+/status/\d+)(?:\?[^\s]*)?`)
)

func rewriteInstagramReels(content string) (string, bool) {
	found := false
	rewritten := instagramReelRegex.ReplaceAllStringFunc(content, func(link string) string {
		found = true
		pathStart := strings.Index(link, "instagram.com/")
		path := link[pathStart+len("instagram.com/"):]
		if queryStart := strings.Index(path, "?"); queryStart != -1 {
			path = path[:queryStart]
		}
		return "https://kkinstagram.com/" + path
	})
	return rewritten, found
}

// deletionFooterBody is the deletion instruction. It names no one: the author is
// already bound to the message by the anchored header, and mentioning them again
// here would ping them for something the bot posted.
const deletionFooterBody = "Sent · the original poster can react with :x: to delete this message."

// smallText prefixes a line so Discord renders it in its small-text style.
func smallText(line string) string {
	return "-# " + line
}

// deletionNotice is the footer every repost ends with, written as small text.
// plainNotice is the same line without the -# marker and is also accepted on
// read, so the check does not depend on Discord keeping that marker in the
// stored message content.
var (
	deletionNotice = "\n\n" + smallText(deletionFooterBody)
	plainNotice    = "\n\n" + deletionFooterBody
)

// legacyDeletionNotice is the wording posted before the footer moved to small
// text and stopped naming the author. Reposts already in a channel stay
// deletable, and this older footer still binds the instruction to that author.
func legacyDeletionNotice(userID string) string {
	return fmt.Sprintf("\n\nNeed to delete this? The original poster <@%s> can react with :x: to delete this message.", userID)
}

func hasDeletionFooter(content, userID string) bool {
	return strings.HasSuffix(content, deletionNotice) ||
		strings.HasSuffix(content, plainNotice) ||
		strings.HasSuffix(content, legacyDeletionNotice(userID))
}

// noPings stops Discord from notifying anyone about a bot message. The repost
// headers carry the original author's ID for authorization and the tracking-free
// message is a reply, but neither should ping the person for the bot's own post.
var noPings = &discordgo.MessageAllowedMentions{
	// An empty parse list, not a nil one, is what tells Discord to parse no
	// mentions at all.
	Parse:       []discordgo.AllowedMentionType{},
	RepliedUser: false,
}

func sendRepost(s *discordgo.Session, channelID, userID, content string) {
	message, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         content + deletionNotice,
		AllowedMentions: noPings,
	})
	if err == nil {
		s.MessageReactionAdd(channelID, message.ID, "❌")
	}
}

// HandleReactionAdd follows the existing reaction-handler pattern: fetch the
// specific message, verify its author and purpose, then act on that message.
func HandleReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if r == nil || r.MessageReaction == nil || s.State == nil || s.State.User == nil ||
		r.UserID == s.State.User.ID || r.Emoji.Name != "❌" || r.Emoji.ID != "" {
		return
	}

	message, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil || !canDeleteRepost(message, s.State.User.ID, r.UserID) {
		return
	}

	s.ChannelMessageDelete(r.ChannelID, r.MessageID)
}

func canDeleteRepost(message *discordgo.Message, botID, userID string) bool {
	if message == nil || message.Author == nil || message.Author.ID != botID || userID == "" {
		return false
	}

	// Match a bot-generated header, never a mention inside copied user text.
	headers := []string{
		fmt.Sprintf("📸 Instagram reel from <@%s>, converted for a better Discord embed:\n", userID),
		fmt.Sprintf("🔗 Converted to fxtwitter for better embeds (original message from <@%s>)\n\n", userID),
		strippedLinksHeader(userID),
	}
	for _, header := range headers {
		if strings.HasPrefix(message.Content, header) {
			return hasDeletionFooter(message.Content, userID)
		}
	}
	return false
}

func HandleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	content := m.Content

	// Replace Instagram reel links with privacy-clean, Discord-friendly embeds.
	if rewritten, found := rewriteInstagramReels(content); found {
		err := s.ChannelMessageDelete(m.ChannelID, m.ID)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Failed to replace the Instagram reel link")
			return
		}

		message := fmt.Sprintf("📸 Instagram reel from <@%s>, converted for a better Discord embed:\n%s", m.Author.ID, rewritten)
		sendRepost(s, m.ChannelID, m.Author.ID, message)
		return
	}

	// Facebook share links can encode information about the sharing account.
	if facebookShareRegex.MatchString(content) {
		err := s.ChannelMessageDelete(m.ChannelID, m.ID)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Failed to remove a potentially identifying Facebook share link")
			return
		}

		warning := fmt.Sprintf("⚠️ <@%s>, Facebook `/share/` links may be associated with your username. Please open the destination post or reel, copy its direct link, and repost that generated direct link instead.", m.Author.ID)
		s.ChannelMessageSend(m.ChannelID, warning)
		return
	}

	// Handle X/Twitter links
	if twitterMatch := twitterRegex.FindString(content); twitterMatch != "" {
		// Extract the path part of the URL (everything after the domain)
		pathStart := strings.Index(twitterMatch, ".com/")
		if pathStart != -1 {
			path := twitterMatch[pathStart+5:]
			// Remove any query parameters
			if queryStart := strings.Index(path, "?"); queryStart != -1 {
				path = path[:queryStart]
			}
			// Create fxtwitter URL
			fxURL := "https://fxtwitter.com/" + path

			// Delete the original message
			err := s.ChannelMessageDelete(m.ChannelID, m.ID)
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, "Failed to remove message with X/Twitter link")
				return
			}

			// Construct the message
			message := fmt.Sprintf("🔗 Converted to fxtwitter for better embeds (original message from <@%s>)\n\n", m.Author.ID)

			// Add any additional content from the original message. Any other link
			// the poster included is cleaned here too, so a repost never carries
			// analytics along with it.
			extraContent := strings.TrimSpace(strings.Replace(content, twitterMatch, "", 1))
			if extraContent != "" {
				message += stripTrackingParams(extraContent) + "\n"
			}
			message += fxURL

			// Send the message
			sendRepost(s, m.ChannelID, m.Author.ID, message)
			return
		}
	}

	// Warn about potentially unsafe TikTok links
	if tiktokRegex.MatchString(content) && !strings.Contains(content, "/video/") {
		warning := "⚠️ Warning: This TikTok link format may expose your profile. Consider using the full video URL format (e.g., https://www.tiktok.com/@username/video/123456) instead."
		// Send warning as a reply to the original message
		_, err := s.ChannelMessageSendReply(m.ChannelID, warning, &discordgo.MessageReference{
			MessageID: m.ID,
			ChannelID: m.ChannelID,
			GuildID:   m.GuildID,
		})
		if err != nil {
			// Fallback to regular message if reply fails
			s.ChannelMessageSend(m.ChannelID, warning)
		}
		return
	}

	// No social-media rewrite applied, so offer the tracking-free copy of any
	// analytics-tagged links. This branch never deletes the original message.
	HandleTrackingLinks(s, m)
}
