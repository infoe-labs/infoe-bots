package mango

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/infoe-labs/infoe-bots/lib/discord"
	"log"
	"strings"
	"time"
)

var (
	UserInQuestion           = "mangostn"
	RoleToAssign             = "1248052023128231937"
	BotUserID                = "1246178480471937054" // Bot user ID
	AllowedChannelsSubString = []string{
		"general", "movebot",
	}
	UserCache = make(map[string]time.Time)
)

const OneDay = 24 * time.Hour

func Mango(DiscordToken string) {
	discord.Call(DiscordToken, Handlers()...)
}

func Handlers() []interface{} {
	return []interface{}{replyBackToMangoReplyWithoutRole, handleReactionAdd}
}

// Check if the channel name contains any of the allowed substrings
func channelNameContainsAllowedSubstring(channelName string) bool {
	channelName = strings.ToLower(channelName)
	for _, substr := range AllowedChannelsSubString {
		if strings.Contains(channelName, substr) {
			return true
		}
	}
	return false
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the authenticated bot has access to.
func replyBackToMangoReplyWithoutRole(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if the user has triggered the bot in the last 24 hours
	if lastTrigger, exists := UserCache[m.Author.ID]; exists && time.Since(lastTrigger) < OneDay {
		return
	}

	// Fetch the channel information
	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		log.Println("Error getting the channel:", err)
		return
	}

	// Check if the channel name contains any of the allowed substrings
	if !channelNameContainsAllowedSubstring(channel.Name) {
		return
	}

	// Check if the user has the role 1248052023128231937
	member, err := s.GuildMember(m.GuildID, m.Author.ID)
	if err != nil {
		log.Println("Error getting the member:", err)
		return
	}

	hasRole := false
	for _, roleID := range member.Roles {
		if roleID == RoleToAssign {
			hasRole = true
			break
		}
	}

	// If the user has the role, do nothing
	if hasRole {
		return
	}

	// User shouldn't have role now
	// Check if the message is a reply
	if m.Message.ReferencedMessage != nil || userMentioned(m.Mentions, UserInQuestion) {

		// Get the original message if it's a reply
		var originalMessage *discordgo.Message
		if m.Message.ReferencedMessage != nil {
			originalMessage, err = s.ChannelMessage(m.ChannelID, m.Message.ReferencedMessage.ID)
			if err != nil {
				log.Println("Error getting the original message:", err)
				return
			}
		}

		// Check whether the reply or mention targets the configured user.
		if (originalMessage != nil && originalMessage.Author.Username == UserInQuestion) || userMentioned(m.Mentions, UserInQuestion) {
			customMessage := fmt.Sprintf(`
			Hey, %s!
You're attempting to contact our neighborhood new grad, %s. 
			
FYI, there's a possibility %s blocked you. If you're unable to react to one of %s's messages then you've been blocked. 
			
To prevent abuse, we limit this notice to once per day. Unsubscribe to future notifications by reacting to this message!
`, m.Author.Username, UserInQuestion, UserInQuestion, UserInQuestion)
			reply := &discordgo.MessageSend{
				Content:   customMessage,
				Reference: &discordgo.MessageReference{MessageID: m.ID},
			}
			_, err := s.ChannelMessageSendComplex(m.ChannelID, reply)
			if err != nil {
				log.Println("Error sending the message:", err)
				return
			}

			// Update the cache with the current timestamp
			UserCache[m.Author.ID] = time.Now()
			return
		}
	}
}

// This function will be called when a reaction is added to a message
func handleReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	// Get the message to check if it's authored by the bot
	message, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		log.Println("Error getting the message:", err)
		return
	}

	// Check if the message is authored by the bot
	if message.Author.ID == BotUserID {
		// Add the role to the user who reacted
		err := s.GuildMemberRoleAdd(r.GuildID, r.UserID, RoleToAssign)
		if err != nil {
			log.Println("Error adding role:", err)
		}
	}
}

// Check if UserInQuestion is mentioned in the message
func userMentioned(mentions []*discordgo.User, username string) bool {
	for _, mention := range mentions {
		if strings.ToLower(mention.Username) == strings.ToLower(username) {
			return true
		}
	}
	return false
}
