package cli

import (
	"github.com/infoe-labs/infoe-bots/lib/discord"
	"github.com/infoe-labs/infoe-bots/lib/messagepreview"
	"github.com/spf13/cobra"
)

var messagePreviewCmd = &cobra.Command{
	Use:   "messagepreview",
	Short: "Reply to Discord message links with author-attributed previews",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		discord.Call(DiscordToken, messagepreview.HandleMessage)
	},
}

func init() {
	rootCmd.AddCommand(messagePreviewCmd)
}
