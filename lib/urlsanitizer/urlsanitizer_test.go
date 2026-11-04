package urlsanitizer

import (
	"encoding/json"
	"github.com/bwmarrin/discordgo"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRewriteInstagramReels(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{
			name:    "reel with tracking",
			input:   "watch https://www.instagram.com/reel/ABC123/?igsh=secret please",
			want:    "watch https://kkinstagram.com/reel/ABC123/ please",
			changed: true,
		},
		{
			name:    "multiple reels",
			input:   "https://instagram.com/reel/one/ https://www.instagram.com/reels/two?x=1",
			want:    "https://kkinstagram.com/reel/one/ https://kkinstagram.com/reels/two",
			changed: true,
		},
		{
			name:    "ordinary text",
			input:   "nothing to rewrite",
			want:    "nothing to rewrite",
			changed: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, changed := rewriteInstagramReels(test.input)
			if got != test.want || changed != test.changed {
				t.Fatalf("rewriteInstagramReels() = (%q, %v), want (%q, %v)", got, changed, test.want, test.changed)
			}
		})
	}
}

func TestFacebookShareRegex(t *testing.T) {
	for _, link := range []string{
		"https://facebook.com/share/r/abc123/",
		"https://www.facebook.com/share/p/abc123/?tracking=value",
		"https://m.facebook.com/share/v/abc123/",
	} {
		if !facebookShareRegex.MatchString(link) {
			t.Errorf("expected Facebook share link to match: %s", link)
		}
	}

	if facebookShareRegex.MatchString("https://www.facebook.com/example/posts/123") {
		t.Error("direct Facebook post link must not match")
	}
}

func TestCanDeleteRepost(t *testing.T) {
	instagram := "📸 Instagram reel from <@123>, converted for a better Discord embed:\ntext"
	twitter := "🔗 Converted to fxtwitter for better embeds (original message from <@123>)\n\ntext"
	for _, tt := range []struct {
		name, author, user, content string
		want                        bool
	}{
		{"instagram owner", "bot", "123", instagram + deletionNotice, true},
		{"twitter owner", "bot", "123", twitter + deletionNotice, true},
		{"footer written without the small-text marker", "bot", "123", instagram + plainNotice, true},
		{"owner of a repost posted before the small-text footer", "bot", "123", instagram + legacyDeletionNotice("123"), true},
		{"other user", "bot", "456", instagram + deletionNotice, false},
		{"other bot", "other-bot", "123", instagram + deletionNotice, false},
		{"user impersonates repost", "123", "123", instagram + deletionNotice, false},
		{"unrelated bot message", "bot", "123", "hello" + deletionNotice, false},
		{"stripped repost belonging to someone else", "bot", "456", strippedLinksHeader("123") + "https://example.com/a" + deletionNotice, false},
		{"legacy footer names another user", "bot", "123", instagram + legacyDeletionNotice("456"), false},
		{"missing footer", "bot", "123", instagram, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			message := &discordgo.Message{Author: &discordgo.User{ID: tt.author}, Content: tt.content}
			if got := canDeleteRepost(message, "bot", tt.user); got != tt.want {
				t.Fatalf("canDeleteRepost() = %v, want %v", got, tt.want)
			}
		})
	}
}

type reactionTransport func(*http.Request) (*http.Response, error)

func (f reactionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReactionDeletesOnlyOriginalAuthorsRepost(t *testing.T) {
	for _, tt := range []struct {
		name, user, emoji, emojiID string
		wantDelete                 bool
	}{
		{"owner cross", "123", "❌", "", true},
		{"someone else cross", "456", "❌", "", false},
		{"owner other emoji", "123", "👍", "", false},
		{"bot own reaction", "bot", "❌", "", false},
		// A server may upload a custom emoji literally named ":x:"; it must not
		// be treated as the Unicode ❌ that authorizes deletion.
		{"custom emoji named cross", "123", "❌", "1234567890", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deleted := false
			s, err := discordgo.New("Bot test-token")
			if err != nil {
				t.Fatal(err)
			}
			s.State.User = &discordgo.User{ID: "bot"}
			s.Client = &http.Client{Transport: reactionTransport(func(r *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(r.URL.Path, "/channels/channel/messages/repost") {
					t.Fatalf("unexpected request path: %s", r.URL.Path)
				}
				body := ""
				status := http.StatusOK
				switch r.Method {
				case http.MethodGet:
					payload, _ := json.Marshal(&discordgo.Message{ID: "repost", Author: &discordgo.User{ID: "bot"}, Content: "📸 Instagram reel from <@123>, converted for a better Discord embed:\ntext" + deletionNotice})
					body = string(payload)
				case http.MethodDelete:
					deleted = true
					status = http.StatusNoContent
				default:
					t.Fatalf("unexpected method: %s", r.Method)
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			HandleReactionAdd(s, &discordgo.MessageReactionAdd{MessageReaction: &discordgo.MessageReaction{UserID: tt.user, ChannelID: "channel", MessageID: "repost", Emoji: discordgo.Emoji{Name: tt.emoji, ID: tt.emojiID}}})
			if deleted != tt.wantDelete {
				t.Fatalf("deleted = %v, want %v", deleted, tt.wantDelete)
			}
		})
	}
}
