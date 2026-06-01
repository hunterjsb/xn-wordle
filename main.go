//go:build !lambda

// Command xn-wordle runs the bot as a long-lived gateway process (slash commands
// + periodic refresh). The scheduled-post Lambda build (-tags lambda) uses lambda.go
// instead; shared logic lives in config.go / wordle.go / commands.go.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

// refreshInterval controls how often the channel is re-scanned for new daily results.
const refreshInterval = 15 * time.Minute

func main() {
	// One-shot: run the scheduled-post path locally (same code the Lambda runs),
	// then exit. Handy for testing the daily post or posting on demand.
	if os.Getenv("WORDLE_POST_ONCE") == "1" {
		if err := runScheduledPost(); err != nil {
			fmt.Println("scheduled post failed:", err)
		}
		return
	}

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		fmt.Println("Error: DISCORD_TOKEN is not set. Bot cannot start.")
		return
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	board = NewLeaderboard()

	// Dry-run mode: backfill, print the standings to stdout, and exit without
	// registering commands or holding the connection open. Handy for local checks.
	if os.Getenv("WORDLE_PRINT_ONLY") == "1" {
		if err := dg.Open(); err != nil {
			fmt.Println("error opening connection,", err)
			return
		}
		defer dg.Close()
		if err := board.Refresh(dg, channelID); err != nil {
			fmt.Printf("scan failed: %v\n", err)
			return
		}
		fmt.Printf("Scanned %d days, %d players.\n\n", board.Days(), board.Players())
		if table, ok := renderLeaderboard(dg); ok {
			fmt.Print(table)
		} else {
			fmt.Println("(no results recorded yet)")
		}
		return
	}

	// Register slash command handler. We only need guild + REST reads — no
	// privileged Message Content intent, since history is read via the REST API.
	dg.AddHandler(interactionCreate)
	dg.Identify.Intents = discordgo.IntentsGuilds

	if err := dg.Open(); err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Clean up stale global commands (we use guild commands only).
	if globalCmds, err := dg.ApplicationCommands(dg.State.User.ID, ""); err == nil && len(globalCmds) > 0 {
		fmt.Printf("Cleaning up %d stale global commands...\n", len(globalCmds))
		for _, cmd := range globalCmds {
			_ = dg.ApplicationCommandDelete(dg.State.User.ID, "", cmd.ID)
		}
	}

	// Register slash commands.
	for _, cmd := range slashCommands {
		created, err := dg.ApplicationCommandCreate(dg.State.User.ID, guildID, cmd)
		if err != nil {
			fmt.Printf("Error registering slash command '%s': %v\n", cmd.Name, err)
		} else {
			fmt.Printf("Registered slash command: /%s\n", created.Name)
		}
	}

	// Initial backfill of the full channel history, then refresh periodically.
	go func() {
		if err := board.Refresh(dg, channelID); err != nil {
			fmt.Printf("Initial Wordle scan failed: %v\n", err)
			return
		}
		fmt.Printf("Initial Wordle scan complete: %d days, %d players.\n", board.Days(), board.Players())
	}()
	go periodicRefresh(dg)

	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()
}

// interactionCreate routes slash command interactions to their handlers.
func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if handler, ok := slashCommandHandlers[i.ApplicationCommandData().Name]; ok {
		handler(s, i)
	}
}

// periodicRefresh re-scans the channel on a fixed interval to pick up new daily results.
func periodicRefresh(s *discordgo.Session) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := board.Refresh(s, channelID); err != nil {
			fmt.Printf("Periodic Wordle scan failed: %v\n", err)
		}
	}
}
