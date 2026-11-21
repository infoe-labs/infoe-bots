package messagepreview

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/bwmarrin/discordgo"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHandleMessage(t *testing.T) {
	const link = "https://discord.com/channels/100/200/300"
	for _, tt := range []struct {
		name, content         string
		bot, webhook          bool
		unavailable           bool
		wantReads, wantEmbeds int
	}{
		{name: "reply", content: link, wantReads: 1, wantEmbeds: 1},
		{name: "cross channel core", content: "https://discord.com/channels/100/201/300", wantReads: 1, wantEmbeds: 1},
		{name: "duplicate", content: link + " " + link, wantReads: 1, wantEmbeds: 1},
		{name: "variants", content: "https://ptb.discord.com/channels/100/200/301?x=1 https://canary.discord.com/channels/100/200/302 https://discordapp.com/channels/100/200/303", wantReads: 3, wantEmbeds: 3},
		{name: "bounded", content: link + " https://discord.com/channels/100/200/301 https://discord.com/channels/100/200/302 https://discord.com/channels/100/200/303", wantReads: 3, wantEmbeds: 3},
		{name: "bot", content: link, bot: true},
		{name: "webhook", content: link, webhook: true},
		{name: "other guild", content: "https://discord.com/channels/101/200/300"},
		{name: "DM link", content: "https://discord.com/channels/@me/200/300"},
		{name: "suppressed", content: "<" + link + ">"},
		{name: "invalid message ID", content: link + "abc"},
		{name: "embedded URL", content: "https://example.invalid/" + link},
		{name: "unavailable", content: link, unavailable: true, wantReads: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := discordgo.New("Bot test-token")
			if err != nil {
				t.Fatal(err)
			}
			reads, sends := 0, 0
			s.Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				body := `{}`
				switch r.Method {
				case "GET":
					if strings.HasSuffix(r.URL.Path, "/channels/200") || strings.HasSuffix(r.URL.Path, "/channels/201") {
						body = `{"guild_id":"100","type":0,"permission_overwrites":[{"id":"100","type":0,"allow":"0","deny":"66560"},{"id":"500","type":0,"allow":"66560","deny":"0"}]}`
						break
					}
					if !strings.Contains(r.URL.Path, "/channels/200/messages/") && !strings.Contains(r.URL.Path, "/channels/201/messages/") {
						t.Fatalf("unexpected GET path")
					}
					reads++
					if tt.unavailable {
						return nil, fmt.Errorf("unavailable")
					}
					body = `{"id":"300","content":"Hello @everyone <@400>","author":{"id":"400","username":"example","global_name":"Example","avatar":"abc"}}`
				case "POST":
					sends++
					if !strings.HasSuffix(r.URL.Path, "/channels/200/messages") {
						t.Fatal("wrong reply channel")
					}
					var payload discordgo.MessageSend
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if len(payload.Embeds) != tt.wantEmbeds {
						t.Errorf("embeds = %d", len(payload.Embeds))
					}
					if payload.Reference == nil || payload.Reference.MessageID != "500" {
						t.Error("missing reply reference")
					}
					mentions := payload.AllowedMentions
					if mentions == nil || mentions.Parse == nil || len(mentions.Parse) != 0 || mentions.RepliedUser || len(mentions.Users) != 0 || len(mentions.Roles) != 0 {
						t.Error("mentions must be disabled")
					}
					if len(payload.Embeds) > 0 && (payload.Embeds[0].Author.Name != "Example" || payload.Embeds[0].Author.IconURL == "") {
						t.Error("missing author attribution")
					}
				default:
					t.Fatalf("unexpected method %s", r.Method)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			webhook := ""
			if tt.webhook {
				webhook = "600"
			}
			HandleMessage(s, &discordgo.MessageCreate{Message: &discordgo.Message{ID: "500", ChannelID: "200", GuildID: "100", Content: tt.content, Author: &discordgo.User{ID: "400", Bot: tt.bot}, WebhookID: webhook}})
			if reads != tt.wantReads {
				t.Errorf("reads = %d, want %d", reads, tt.wantReads)
			}
			wantSends := 0
			if tt.wantEmbeds > 0 {
				wantSends = 1
			}
			if sends != wantSends {
				t.Errorf("sends = %d, want %d", sends, wantSends)
			}
		})
	}
}

func TestMatchingAudience(t *testing.T) {
	everyone := func(deny int64) *discordgo.PermissionOverwrite {
		return &discordgo.PermissionOverwrite{ID: "100", Type: discordgo.PermissionOverwriteTypeRole, Deny: deny}
	}
	core := func(allow int64) *discordgo.PermissionOverwrite {
		return &discordgo.PermissionOverwrite{ID: "500", Type: discordgo.PermissionOverwriteTypeRole, Allow: allow}
	}
	member := func(deny int64) *discordgo.PermissionOverwrite {
		return &discordgo.PermissionOverwrite{ID: "600", Type: discordgo.PermissionOverwriteTypeMember, Deny: deny}
	}
	restricted := func() []*discordgo.PermissionOverwrite {
		return []*discordgo.PermissionOverwrite{everyone(readPermissions), core(readPermissions)}
	}
	for _, tt := range []struct {
		name                    string
		from, to                []*discordgo.PermissionOverwrite
		sourceType, targetType  discordgo.ChannelType
		sourceNSFW, targetNSFW  bool
		otherGuild, unavailable bool
		want                    bool
	}{
		{name: "public channels", want: true},
		{name: "matching core access", from: restricted(), to: restricted(), want: true},
		{name: "ignore ordering", from: restricted(), to: []*discordgo.PermissionOverwrite{core(readPermissions), everyone(readPermissions)}, want: true},
		{name: "ignore send permissions", from: restricted(), to: []*discordgo.PermissionOverwrite{everyone(readPermissions), core(readPermissions | discordgo.PermissionSendMessages)}, want: true},
		{name: "ignore neutral overrides", to: []*discordgo.PermissionOverwrite{core(discordgo.PermissionSendMessages)}, want: true},
		{name: "core missing at destination", from: restricted(), to: []*discordgo.PermissionOverwrite{everyone(readPermissions)}},
		{name: "core missing at source", from: []*discordgo.PermissionOverwrite{everyone(readPermissions)}, to: restricted()},
		{name: "moderator source to core", from: []*discordgo.PermissionOverwrite{everyone(readPermissions), {ID: "700", Type: discordgo.PermissionOverwriteTypeRole, Allow: readPermissions}}, to: restricted()},
		{name: "history differs", from: restricted(), to: []*discordgo.PermissionOverwrite{everyone(readPermissions), core(discordgo.PermissionViewChannel)}},
		{name: "member denial differs", from: append(restricted(), member(discordgo.PermissionViewChannel)), to: restricted()},
		{name: "matching member denial", from: append(restricted(), member(discordgo.PermissionViewChannel)), to: append(restricted(), member(discordgo.PermissionViewChannel)), want: true},
		{name: "role and member IDs are distinct", from: []*discordgo.PermissionOverwrite{core(readPermissions)}, to: []*discordgo.PermissionOverwrite{{ID: "500", Type: discordgo.PermissionOverwriteTypeMember, Allow: readPermissions}}},
		{name: "source thread", sourceType: discordgo.ChannelTypeGuildPublicThread},
		{name: "destination thread", targetType: discordgo.ChannelTypeGuildPrivateThread},
		{name: "different guild", otherGuild: true},
		{name: "NSFW to safe", sourceNSFW: true},
		{name: "NSFW to NSFW", sourceNSFW: true, targetNSFW: true, want: true},
		{name: "safe to NSFW", targetNSFW: true, want: true},
		{name: "channel lookup failed", unavailable: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := &discordgo.Channel{ID: "200", GuildID: "100", Type: tt.sourceType, PermissionOverwrites: tt.from, NSFW: tt.sourceNSFW}
			target := &discordgo.Channel{ID: "201", GuildID: "100", Type: tt.targetType, PermissionOverwrites: tt.to, NSFW: tt.targetNSFW}
			if tt.otherGuild {
				target.GuildID = "101"
			}
			s, err := discordgo.New("Bot test-token")
			if err != nil {
				t.Fatal(err)
			}
			s.StateEnabled = false
			s.Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				if tt.unavailable {
					return nil, fmt.Errorf("unavailable")
				}
				var channel *discordgo.Channel
				switch {
				case strings.HasSuffix(r.URL.Path, "/channels/200"):
					channel = source
				case strings.HasSuffix(r.URL.Path, "/channels/201"):
					channel = target
				default:
					t.Fatal("unexpected request")
				}
				body, err := json.Marshal(channel)
				if err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
			})}
			if got := matchingAudience(s, "100", "200", "201"); got != tt.want {
				t.Errorf("allowed = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPreview(t *testing.T) {
	m := &discordgo.Message{
		Content: strings.Repeat("😀", 1000),
		Author:  &discordgo.User{ID: "400", Username: "example", GlobalName: "Example"},
		Member:  &discordgo.Member{Nick: "Nickname", Avatar: "abc"},
		Attachments: []*discordgo.MessageAttachment{
			{Filename: "SPOILER_image.png", ContentType: "image/png", URL: "https://example.invalid/spoiler"},
			{Filename: "image.png", ContentType: "image/png", URL: "https://example.invalid/image"},
		},
	}
	embed := preview(m, "https://example.invalid/message", "100")
	if len(utf16.Encode([]rune(embed.Description))) > 1200 || !strings.HasSuffix(embed.Description, "…") {
		t.Error("description exceeds limit or lacks truncation marker")
	}
	if embed.Author.Name != "Nickname" || !strings.Contains(embed.Author.IconURL, "/guilds/100/users/400/avatars/abc") {
		t.Error("guild author display is missing")
	}
	if embed.Image == nil || embed.Image.URL != "https://example.invalid/image" {
		t.Error("wrong image or spoiler exposed")
	}
	if m.Member.User != nil {
		t.Error("preview mutated the original message")
	}
	m.Content = ""
	m.Attachments = nil
	embed = preview(m, "https://example.invalid/message", "100")
	if embed.Description == "" || embed.Image != nil {
		t.Error("empty message should produce a text fallback")
	}
}

func TestPrivateSourceIsNotFetched(t *testing.T) {
	s, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatal(err)
	}
	s.StateEnabled = false
	s.Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/channels/201"):
			body = `{"guild_id":"100","type":0}`
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/channels/200"):
			body = `{"guild_id":"100","type":0,"permission_overwrites":[{"id":"500","type":0,"allow":"66560","deny":"0"}]}`
		default:
			t.Fatal("private source content must not be fetched or posted")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	HandleMessage(s, &discordgo.MessageCreate{Message: &discordgo.Message{
		ID: "500", GuildID: "100", ChannelID: "200", Author: &discordgo.User{ID: "400"},
		Content: "https://discord.com/channels/100/201/300",
	}})
}
