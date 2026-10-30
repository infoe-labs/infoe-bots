package cli

import (
	"fmt"
	"github.com/infoe-labs/infoe-bots/lib/ping"
	"github.com/spf13/cobra"
)

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Replies with pong if someone replies with ping",
	Long:  "N/A",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running ping function")
		ping.Ping(DiscordToken)
	},
}

func init() {
	rootCmd.AddCommand(pingCmd)
}
