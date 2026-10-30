package cli

import (
	"fmt"
	mango "github.com/infoe-labs/infoe-bots/lib/mangostn"
	"github.com/spf13/cobra"
)

var mangoCmd = &cobra.Command{
	Use:   "mango",
	Short: "Run the opt-in legacy mango joke bot",
	Long:  "A preserved legacy joke subcommand. It is not registered by the all command and runs only when explicitly invoked.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running legacy mango joke bot")
		mango.Mango(DiscordToken)
	},
}

func init() {
	rootCmd.AddCommand(mangoCmd)
}
