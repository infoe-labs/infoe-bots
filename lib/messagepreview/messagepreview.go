// Package messagepreview replies to Discord message links without retaining content.
package messagepreview

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const publicPermissions = discordgo.PermissionViewChannel | discordgo.PermissionReadMessageHistory

var messageLink = regexp.MustCompile(`https://(?:(?:canary|ptb)\.)?discord(?:app)?\.com/channels/([0-9]+)/([0-9]+)/([0-9]+)(?:[/?#][^\s<>]*)?`)

// HandleMessage previews at most three distinct links, with no mention pings.
func HandleMessage(s *discordgo.Session, event *discordgo.MessageCreate) {
	if event == nil || event.Message == nil {
		return
	}
	m := event.Message
	if m.Author == nil || m.Author.Bot || m.WebhookID != "" || m.GuildID == "" {
		return
	}
	links := messageLink.FindAllStringSubmatchIndex(m.Content, -1)
	seen := make(map[string]bool)
	embeds := make([]*discordgo.MessageEmbed, 0, 3)
	attempts := 0
	for _, match := range links {
		// Respect Discord's explicit <link> suppression convention, and don't match
		// a URL embedded inside another URL or hostname.
		start, end := match[0], match[1]
		if start > 0 && !strings.ContainsRune(" \n\t([", rune(m.Content[start-1])) {
			continue
		}
		if end < len(m.Content) && strings.ContainsRune("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-", rune(m.Content[end])) {
			continue
		}
		guild := m.Content[match[2]:match[3]]
		channel := m.Content[match[4]:match[5]]
		id := m.Content[match[6]:match[7]]
		key := channel + "/" + id
		if guild != m.GuildID || seen[key] {
			continue
		}
		seen[key] = true
		attempts++
		if attempts > 3 {
			break
		}
		if channel != m.ChannelID && !publicSource(s, guild, channel, m.ChannelID) {
			continue
		}
		original, err := s.ChannelMessage(channel, id)
		if err != nil || original == nil || original.Author == nil {
			continue
		}
		url := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guild, channel, id)
		embeds = append(embeds, preview(original, url, guild))
	}
	if len(embeds) == 0 {
		return
	}
	_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Embeds:          embeds,
		Reference:       &discordgo.MessageReference{MessageID: m.ID, ChannelID: m.ChannelID, GuildID: m.GuildID},
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}, RepliedUser: false},
	})
	if err != nil {
		fmt.Println("Discord message preview send failed")
	}
}

// Cross-channel previews require an ordinary channel visible to everyone,
// without any role/member denial of viewing or reading history. Threads are intentionally excluded here:
// parent permissions alone do not establish private-thread membership.
func publicSource(s *discordgo.Session, guild, source, destination string) bool {
	channel, err := s.Channel(source)
	if err != nil || channel.GuildID != guild || (channel.Type != discordgo.ChannelTypeGuildText && channel.Type != discordgo.ChannelTypeGuildNews) {
		return false
	}
	if channel.NSFW {
		target, err := s.Channel(destination)
		if err != nil || !target.NSFW {
			return false
		}
	}
	roles, err := s.GuildRoles(guild)
	if err != nil {
		return false
	}
	var permissions int64
	foundEveryone := false
	for _, role := range roles {
		if role.ID == guild {
			permissions = role.Permissions
			foundEveryone = true
			break
		}
	}
	if !foundEveryone {
		return false
	}
	for _, overwrite := range channel.PermissionOverwrites {
		if overwrite.Deny&publicPermissions != 0 {
			return false
		}
		if overwrite.Type == discordgo.PermissionOverwriteTypeRole && overwrite.ID == guild {
			permissions = (permissions &^ overwrite.Deny) | overwrite.Allow
		}
	}
	return permissions&publicPermissions == publicPermissions
}

func truncate(text string, limit int) string {
	// Counting UTF-16 units also covers Discord's limits for non-BMP characters.
	units := 0
	for index, r := range text {
		cost := 1
		if r > 0xffff {
			cost = 2
		}
		if units+cost > limit-1 {
			return text[:index] + "…"
		}
		units += cost
	}
	return text
}

func preview(m *discordgo.Message, url, guild string) *discordgo.MessageEmbed {
	name := m.Author.GlobalName
	if name == "" {
		name = m.Author.Username
	}
	avatar := m.Author.AvatarURL("128")
	if m.Member != nil {
		if m.Member.Nick != "" {
			name = m.Member.Nick
		}
		if m.Member.Avatar != "" {
			member := *m.Member
			member.User = m.Author
			member.GuildID = guild
			avatar = member.AvatarURL("128")
		}
	}
	description := m.Content
	if description == "" {
		description = "[No text content]"
	}
	embed := &discordgo.MessageEmbed{
		Title: "View original message", URL: url, Description: truncate(description, 1200), Color: 0x5865f2,
		Author: &discordgo.MessageEmbedAuthor{Name: truncate(name, 256), IconURL: avatar},
		Footer: &discordgo.MessageEmbedFooter{Text: "Discord message preview · snapshot"},
	}
	if !m.Timestamp.IsZero() {
		embed.Timestamp = m.Timestamp.UTC().Format("2006-01-02T15:04:05Z")
	}
	for _, attachment := range m.Attachments {
		if attachment != nil && strings.HasPrefix(attachment.ContentType, "image/") && !strings.HasPrefix(attachment.Filename, "SPOILER_") {
			embed.Image = &discordgo.MessageEmbedImage{URL: attachment.URL}
			break
		}
	}
	return embed
}
