package cli

import (
	"fmt"
	mango "github.com/infoe-labs/infoe-bots/lib/mangostn"
	"github.com/spf13/cobra"
)

var mangoCmd = &cobra.Command{
	Use:   "mango",
	Short: "Replies if someone replies to mango",
	Long:  "N/A",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running mango function")
		mango.Mango(DiscordToken)
	},
}

func init() {
	rootCmd.AddCommand(mangoCmd)
}
