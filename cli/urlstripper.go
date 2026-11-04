package cli

import (
	"fmt"

	"github.com/infoe-labs/infoe-bots/lib/discord"
	"github.com/infoe-labs/infoe-bots/lib/urlsanitizer"
	"github.com/spf13/cobra"
)

var urlStripperCmd = &cobra.Command{
	Use:   "urlstripper",
	Short: "Posts tracking-free copies of links without deleting the original",
	Long: "No-analytics mode: detects tracking parameters such as utm_*, fbclid and gclid, then adds a new " +
		"message in the same channel with the same links stripped of them. The original message is left alone.",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running URL tracking stripper")
		discord.Call(DiscordToken, urlsanitizer.HandleTrackingLinks, urlsanitizer.HandleReactionAdd)
	},
}

func init() {
	rootCmd.AddCommand(urlStripperCmd)
}
