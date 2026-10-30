package ping

import (
	"github.com/bwmarrin/discordgo"
	"github.com/infoe-labs/infoe-bots/lib/discord"
)

func Ping(DiscordToken string) {
	discord.Call(DiscordToken, Handlers()...)
}

func Handlers() []interface{} {
	return []interface{}{messagePingBack}
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the authenticated bot has access to.
func messagePingBack(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// If the message is "ping_abcdefg" reply with "Pong!"
	if m.Content == "ping_abcdefg" {
		s.ChannelMessageSend(m.ChannelID, "Pong!")
	}
}
