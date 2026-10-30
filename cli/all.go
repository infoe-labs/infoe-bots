package cli

import (
	"fmt"

	"github.com/infoe-labs/infoe-bots/lib/discord"
	"github.com/infoe-labs/infoe-bots/lib/llmpromoter"
	mango "github.com/infoe-labs/infoe-bots/lib/mangostn"
	"github.com/infoe-labs/infoe-bots/lib/ping"
	"github.com/infoe-labs/infoe-bots/lib/urlsanitizer"
	"github.com/spf13/cobra"
)

var allCmd = &cobra.Command{
	Use:   "all",
	Short: "Run all bot features in one Discord session",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running all bot features")

		handlers := append([]interface{}{}, ping.Handlers()...)
		handlers = append(handlers, mango.Handlers()...)
		handlers = append(handlers,
			urlsanitizer.HandleMessage,
			llmpromoter.HandleMessage,
			llmpromoter.HandleReactionAdd,
		)

		discord.Call(DiscordToken, handlers...)
	},
}

func init() {
	rootCmd.AddCommand(allCmd)
}
