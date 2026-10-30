package discord

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"os/signal"
	"syscall"
)

func Call(DiscordToken string, handlers ...interface{}) {
	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + DiscordToken)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	// Register each handler passed in as a callback for appropriate events.
	for _, handler := range handlers {
		dg.AddHandler(handler)
	}

	// In this example, we only care about receiving message events and reactions.
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuildMessageReactions

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}
