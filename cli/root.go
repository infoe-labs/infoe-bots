package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/infoe-labs/infoe-bots/lib/sqlcache"
	"github.com/spf13/cobra"
)

var (
	DiscordToken     string
	discordTokenFile string
	Version          = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "infoe",
	Short:   "main CLI for root command",
	Long:    "TODO",
	Version: Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if discordTokenFile == "" {
			return fmt.Errorf("--discord-token-file is required")
		}
		info, err := os.Stat(discordTokenFile)
		if err != nil {
			return fmt.Errorf("cannot read Discord token file metadata")
		}
		if info.Mode().Perm() != 0600 {
			return fmt.Errorf("Discord token file must have mode 0600")
		}
		token, err := os.ReadFile(discordTokenFile)
		if err != nil {
			return fmt.Errorf("cannot read Discord token file")
		}
		DiscordToken = strings.TrimSpace(string(token))
		if strings.TrimSpace(DiscordToken) == "" {
			return fmt.Errorf("Discord token file is empty")
		}
		if err := sqlcache.Initialize(DiscordToken); err != nil {
			return fmt.Errorf("initialize privacy store: %w", err)
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&discordTokenFile, "discord-token-file", "", "path to a file containing the discord api token")
}
