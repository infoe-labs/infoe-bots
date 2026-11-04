package urlsanitizer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestStrippedTrackingLinks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "analytics campaign stripped to the bare article",
			input: "read this https://cybernews.com/security/europe-supermarket-refrigerator-hacks/?utm_source=cn_ytcommunity&utm_medium=social&utm_campaign=cybernews&utm_content=post&source=cn_ytcommunity&medium=social&campaign=cybernews&content=post",
			want:  []string{"https://cybernews.com/security/europe-supermarket-refrigerator-hacks/"},
		},
		{
			name:  "real query parameters survive",
			input: "https://example.com/search?q=go+language&id=7",
			want:  nil,
		},
		{
			name:  "generic names survive without a utm parameter",
			input: "https://example.com/article?source=newsletter&medium=weekly",
			want:  nil,
		},
		{
			name:  "generic names go when the utm twins are present",
			input: "https://example.com/article?utm_source=x&source=x&medium=y&campaign=z&term=t&content=c&page=2",
			want:  []string{"https://example.com/article?page=2"},
		},
		{
			name:  "click ids and referrer tags",
			input: "https://example.com/a?fbclid=abc&gclid=def&msclkid=ghi&ttclid=jkl&ref=tw_i",
			want:  []string{"https://example.com/a"},
		},
		{
			name:  "fragment is preserved",
			input: "https://example.com/a?utm_source=x#section-2",
			want:  []string{"https://example.com/a#section-2"},
		},
		{
			name:  "trailing sentence punctuation is not part of the link",
			input: "look at https://example.com/a?utm_source=x, really good.",
			want:  []string{"https://example.com/a"},
		},
		{
			name:  "prose parentheses are trimmed but balanced ones stay",
			input: "see (https://example.com/wiki/Foo_(bar)?utm_source=x)",
			want:  []string{"https://example.com/wiki/Foo_(bar)"},
		},
		{
			name:  "angle-bracket suppressed links are detected",
			input: "<https://example.com/a?gclid=abc>",
			want:  []string{"https://example.com/a"},
		},
		{
			name:  "several links, already-clean ones are not reposted",
			input: "https://example.com/a?utm_source=x and https://example.com/b and https://example.com/c?fbclid=1",
			want:  []string{"https://example.com/a", "https://example.com/c"},
		},
		{
			name:  "duplicate clean results are listed once",
			input: "https://example.com/a?utm_source=x https://example.com/a?utm_source=y",
			want:  []string{"https://example.com/a"},
		},
		{
			name:  "malformed query fails closed",
			input: "https://example.com/a?utm_source=x&%zz",
			want:  nil,
		},
		{
			name:  "no links means nothing to post",
			input: "no links here, just utm_source talk",
			want:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := strippedTrackingLinks(test.input)
			if strings.Join(got, "|") != strings.Join(test.want, "|") {
				t.Fatalf("strippedTrackingLinks() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTrimURLTail(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"https://example.com/a.", "https://example.com/a"},
		{"https://example.com/a?x=1!", "https://example.com/a?x=1"},
		{"https://example.com/a)", "https://example.com/a"},
		{"https://example.com/a(b)", "https://example.com/a(b)"},
		{"https://example.com/a]", "https://example.com/a"},
		{"https://example.com/a[b]", "https://example.com/a[b]"},
		{"https://example.com/", "https://example.com/"},
	} {
		if got := trimURLTail(tt.in); got != tt.want {
			t.Errorf("trimURLTail(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCanDeleteStrippedLinksRepost(t *testing.T) {
	repost := strippedLinksHeader("123") + "https://example.com/a" + deletionNotice
	botMessage := &discordgo.Message{Author: &discordgo.User{ID: "bot"}, Content: repost}

	if !canDeleteRepost(botMessage, "bot", "123") {
		t.Fatal("the original author must be able to delete their tracking-free repost")
	}
	if canDeleteRepost(botMessage, "bot", "456") {
		t.Fatal("another user must not be able to delete someone else's repost")
	}
	impersonation := &discordgo.Message{
		Author:  &discordgo.User{ID: "123"},
		Content: repost,
	}
	if canDeleteRepost(impersonation, "bot", "123") {
		t.Fatal("a user message that copies the repost layout must not be deletable")
	}
}

// postedMessage captures what the bot sent so the assertions can inspect it.
type postedMessage struct {
	content         string
	referenceID     string
	methods         []string
	reactions       []string
	deleted         bool
	allowedMentions *allowedMentions
}

// allowedMentions mirrors the field Discord reads when deciding whether to ping
// anybody, including the pointer on Parse so a nil list and an empty one stay
// distinguishable.
type allowedMentions struct {
	Parse       *[]string `json:"parse"`
	Users       []string  `json:"users"`
	RepliedUser bool      `json:"replied_user"`
}

// requireNoPings checks that the bot asked Discord to parse no mentions, so its
// own repost never notifies the person it is talking about.
func requireNoPings(t *testing.T, posted *postedMessage) {
	t.Helper()
	if posted.allowedMentions == nil {
		t.Fatal("the bot sent a message without allowed_mentions, so Discord would ping the poster")
	}
	if posted.allowedMentions.Parse == nil || len(*posted.allowedMentions.Parse) != 0 {
		t.Fatalf("allowed_mentions.parse must be an explicit empty list, got %#v", posted.allowedMentions.Parse)
	}
	if len(posted.allowedMentions.Users) != 0 || posted.allowedMentions.RepliedUser {
		t.Fatalf("the bot re-enabled mentions: %#v", posted.allowedMentions)
	}
}

func stripTransport(t *testing.T, posted *postedMessage) reactionTransport {
	return reactionTransport(func(r *http.Request) (*http.Response, error) {
		posted.methods = append(posted.methods, r.Method)

		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/channels/channel/messages"):
			var payload struct {
				Content         string                      `json:"content"`
				Reference       *discordgo.MessageReference `json:"message_reference"`
				AllowedMentions *allowedMentions            `json:"allowed_mentions"`
			}
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("unparsable message payload: %v", err)
			}
			posted.content = payload.Content
			posted.allowedMentions = payload.AllowedMentions
			if payload.Reference != nil {
				posted.referenceID = payload.Reference.MessageID
			}
			payloadJSON, _ := json.Marshal(&discordgo.Message{ID: "clean"})
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(payloadJSON))), Request: r}, nil
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/messages/original"):
			posted.deleted = true
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/reactions/"):
			posted.reactions = append(posted.reactions, r.URL.Path)
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return nil, fmt.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
}

func testSession(t *testing.T, posted *postedMessage) *discordgo.Session {
	t.Helper()
	s, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatal(err)
	}
	s.State = &discordgo.State{Ready: discordgo.Ready{User: &discordgo.User{ID: "bot"}}}
	s.Client = &http.Client{Transport: stripTransport(t, posted)}
	return s
}

func TestHandleTrackingLinksAddsMessageWithoutDeleting(t *testing.T) {
	posted := &postedMessage{}
	s := testSession(t, posted)

	HandleTrackingLinks(s, &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "original",
		ChannelID: "channel",
		Author:    &discordgo.User{ID: "123"},
		Content:   "https://cybernews.com/security/europe-supermarket-refrigerator-hacks/?utm_source=cn_ytcommunity&utm_medium=social&medium=social",
	}})

	for _, method := range posted.methods {
		if method == http.MethodDelete {
			t.Fatalf("the no-analytics mode must never delete the original message, methods: %v", posted.methods)
		}
	}
	if posted.content == "" {
		t.Fatal("expected a tracking-free repost")
	}
	if !strings.Contains(posted.content, "https://cybernews.com/security/europe-supermarket-refrigerator-hacks/") {
		t.Fatalf("repost is missing the clean link: %q", posted.content)
	}
	if strings.Contains(posted.content, "utm_") || strings.Contains(posted.content, "medium=") {
		t.Fatalf("repost still carries tracking parameters: %q", posted.content)
	}
	if posted.referenceID != "original" {
		t.Fatalf("repost should reply to the original message, reference = %q", posted.referenceID)
	}
	if !strings.HasPrefix(posted.content, strippedLinksHeader("123")) {
		t.Fatalf("repost is missing the author-bound header: %q", posted.content)
	}
	if !strings.HasSuffix(posted.content, deletionNotice) {
		t.Fatalf("repost is missing the deletion footer: %q", posted.content)
	}
	if !strings.Contains(posted.content, smallText(deletionFooterBody)) {
		t.Fatalf("the footer must be written as small text: %q", posted.content)
	}
	// The header carries the author mention for authorization; nothing else in
	// the message may name a user.
	if mentions := strings.Count(posted.content, "<@"); mentions != 1 {
		t.Fatalf("only the header may mention a user, found %d in %q", mentions, posted.content)
	}
	requireNoPings(t, posted)
	if len(posted.reactions) != 1 {
		t.Fatalf("expected the ❌ deletion reaction to be pre-seeded, got %v", posted.reactions)
	}
}

func TestHandleTrackingLinksStaysSilent(t *testing.T) {
	tests := []struct {
		name    string
		message *discordgo.Message
	}{
		{"bot's own message", &discordgo.Message{Author: &discordgo.User{ID: "bot", Bot: true}, Content: "https://example.com/a?utm_source=x"}},
		{"other bot", &discordgo.Message{Author: &discordgo.User{ID: "999", Bot: true}, Content: "https://example.com/a?utm_source=x"}},
		{"webhook message", &discordgo.Message{Author: &discordgo.User{ID: "999"}, WebhookID: "hook", Content: "https://example.com/a?utm_source=x"}},
		{"link without tracking", &discordgo.Message{Author: &discordgo.User{ID: "123"}, Content: "https://example.com/a?id=7"}},
		{"text that only mentions tracking", &discordgo.Message{Author: &discordgo.User{ID: "123"}, Content: "utm_source is annoying"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			posted := &postedMessage{}
			s := testSession(t, posted)
			test.message.ChannelID = "channel"
			test.message.ID = "original"

			HandleTrackingLinks(s, &discordgo.MessageCreate{Message: test.message})

			if len(posted.methods) != 0 {
				t.Fatalf("expected no Discord writes, got %v", posted.methods)
			}
		})
	}
}

func TestStripTrackingParamsKeepsSurroundingText(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{
			in:   "see [story](https://example.com/a?utm_source=x) nice",
			want: "see [story](https://example.com/a) nice",
		},
		{
			in:   "https://example.com/a?utm_source=x.",
			want: "https://example.com/a.",
		},
		{
			// Nothing was removed, so the text is returned byte for byte.
			in:   "https://example.com/a?id=7. and plain words",
			want: "https://example.com/a?id=7. and plain words",
		},
		{
			in:   "https://example.com/a?fbclid=1 https://example.com/b?q=1+2&gclid=2",
			want: "https://example.com/a https://example.com/b?q=1+2",
		},
	} {
		if got := stripTrackingParams(tt.in); got != tt.want {
			t.Errorf("stripTrackingParams(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestTwitterRepostCleansOtherLinks covers the one path that does delete: the
// text carried over from the original message must not smuggle analytics into
// the repost.
func TestTwitterRepostCleansOtherLinks(t *testing.T) {
	posted := &postedMessage{}
	s := testSession(t, posted)

	HandleMessage(s, &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "original",
		ChannelID: "channel",
		Author:    &discordgo.User{ID: "123"},
		Content:   "look https://x.com/jack/status/20?utm_source=share and https://cybernews.com/security/story/?utm_source=x",
	}})

	if !posted.deleted {
		t.Fatal("the X/Twitter rewrite is expected to keep replacing the original message")
	}
	if !strings.Contains(posted.content, "https://fxtwitter.com/jack/status/20") {
		t.Fatalf("repost is missing the fxtwitter link: %q", posted.content)
	}
	if !strings.Contains(posted.content, "https://cybernews.com/security/story/") {
		t.Fatalf("repost is missing the carried-over link: %q", posted.content)
	}
	if strings.Contains(posted.content, "utm_") {
		t.Fatalf("repost carried tracking parameters into the new message: %q", posted.content)
	}
	requireNoPings(t, posted)
}

// TestHandleMessageFallsThroughToTrackingLinks guards against the social-media
// rewrites and the no-analytics mode both answering the same message.
func TestHandleMessageFallsThroughToTrackingLinks(t *testing.T) {
	posted := &postedMessage{}
	s := testSession(t, posted)

	HandleMessage(s, &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "original",
		ChannelID: "channel",
		Author:    &discordgo.User{ID: "123"},
		Content:   "https://cybernews.com/security/story/?utm_source=x",
	}})

	if posted.content == "" {
		t.Fatal("HandleMessage should reach the no-analytics mode for plain article links")
	}
	for _, method := range posted.methods {
		if method == http.MethodDelete {
			t.Fatalf("the no-analytics mode must not delete, methods: %v", posted.methods)
		}
	}
}
