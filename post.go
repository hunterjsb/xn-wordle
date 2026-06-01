package main

import (
	"fmt"
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
)

// runScheduledPost scans #wordle, builds the leaderboard, and posts it to the
// target channel via REST (no gateway connection). It is the body of the Lambda
// handler, and is also reachable from the gateway binary via WORDLE_POST_ONCE=1
// for local testing / manual runs.
func runScheduledPost() error {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return fmt.Errorf("DISCORD_TOKEN is not set")
	}
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	board = NewLeaderboard()
	if err := board.Refresh(dg, channelID); err != nil {
		return fmt.Errorf("scan #wordle: %w", err)
	}
	log.Printf("scanned %d days, %d players", board.Days(), board.Players())

	container, ok := buildLeaderboardContainer(dg)
	if !ok {
		log.Println("no results yet; nothing to post")
		return nil
	}

	// Post to LEADERBOARD_CHANNEL_ID if set, otherwise back into #wordle.
	postCh := os.Getenv("LEADERBOARD_CHANNEL_ID")
	if postCh == "" {
		postCh = channelID
	}
	msg, err := dg.ChannelMessageSendComplex(postCh, &discordgo.MessageSend{
		Flags:      discordgo.MessageFlagsIsComponentsV2,
		Components: []discordgo.MessageComponent{*container},
	})
	if err != nil {
		return fmt.Errorf("post leaderboard: %w", err)
	}
	log.Printf("posted leaderboard message %s to channel %s", msg.ID, postCh)
	return nil
}
