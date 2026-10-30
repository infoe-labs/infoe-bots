package cli

import (
	"fmt"
	"os"
	"strings"

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
		if DiscordToken != "" && discordTokenFile != "" {
			return fmt.Errorf("use only one of --discord-token or --discord-token-file")
		}
		if discordTokenFile != "" {
			token, err := os.ReadFile(discordTokenFile)
			if err != nil {
				return fmt.Errorf("read Discord token file: %w", err)
			}
			DiscordToken = strings.TrimSpace(string(token))
		}
		if strings.TrimSpace(DiscordToken) == "" {
			return fmt.Errorf("--discord-token or --discord-token-file must be provided")
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
	rootCmd.PersistentFlags().StringVar(&DiscordToken, "discord-token", "", "discord api token")
	rootCmd.PersistentFlags().StringVar(&discordTokenFile, "discord-token-file", "", "path to a file containing the discord api token")
}
