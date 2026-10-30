package cli

import (
	"fmt"
	"github.com/infoe-labs/infoe-bots/lib/discord"
	"github.com/infoe-labs/infoe-bots/lib/urlsanitizer"
	"github.com/spf13/cobra"
)

var urlSanitizerCmd = &cobra.Command{
	Use:   "urlsanitizer",
	Short: "Rewrites social links for safer sharing and better embeds",
	Long:  "Converts Instagram reels to kkinstagram embeds, removes potentially identifying Facebook share links, converts X/Twitter links, and warns about unsafe TikTok links.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running URL sanitizer")
		discord.Call(DiscordToken, urlsanitizer.HandleMessage, urlsanitizer.HandleReactionAdd)
	},
}

func init() {
	rootCmd.AddCommand(urlSanitizerCmd)
}
