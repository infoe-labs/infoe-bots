package urlsanitizer

import "testing"

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
