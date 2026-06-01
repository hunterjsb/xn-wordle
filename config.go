package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Shared config + state, used by both the gateway bot (main.go) and the
// scheduled Lambda (lambda.go).
var (
	channelID string
	guildID   string

	board *Leaderboard
)

func init() {
	// Try loading .env — optional. On Lambda / systemd, env vars come from the
	// platform instead, and the missing file is ignored.
	if envFile := os.Getenv("ENV_FILE"); envFile != "" {
		_ = godotenv.Load(envFile)
	} else {
		_ = godotenv.Load(".env")
	}

	channelID = os.Getenv("WORDLE_CHANNEL_ID")
	if channelID == "" {
		fmt.Println("Warning: WORDLE_CHANNEL_ID is not set.")
	}
	guildID = os.Getenv("DISCORD_GUILD_ID") // Empty string = global commands
}
