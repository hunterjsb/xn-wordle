package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Embed color constants
const (
	colorSuccess = 0x00C853
	colorError   = 0xFF1744
	colorWarning = 0xFFAB00
	colorInfo    = 0x6AAA64 // Wordle green
)

var adminPerm int64 = discordgo.PermissionAdministrator

// Slash command definitions
var slashCommands = []*discordgo.ApplicationCommand{
	{Name: "leaderboard", Description: "Show the XAN NATION Wordle leaderboard"},
	{Name: "wordlerefresh", Description: "Force a re-scan of the #wordle channel", DefaultMemberPermissions: &adminPerm},
}

// Handler map routes slash command names to handler functions
var slashCommandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
	"leaderboard":   handleLeaderboard,
	"wordlerefresh": handleRefresh,
}

// --- Embed helpers ---

func successEmbed(title, desc string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{Title: title, Description: desc, Color: colorSuccess, Timestamp: time.Now().Format(time.RFC3339)}
}

func errorEmbed(title, desc string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{Title: title, Description: desc, Color: colorError, Timestamp: time.Now().Format(time.RFC3339)}
}

func warningEmbed(title, desc string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{Title: title, Description: desc, Color: colorWarning, Timestamp: time.Now().Format(time.RFC3339)}
}

func infoEmbed(title, desc string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{Title: title, Description: desc, Color: colorInfo, Timestamp: time.Now().Format(time.RFC3339)}
}

// respondEmbed sends an immediate interaction response with an embed.
func respondEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

// deferResponse sends a deferred interaction response (shows "thinking...").
func deferResponse(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}

// followupEmbed sends a followup message with an embed after a deferred response.
func followupEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
	s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	})
}

// --- Slash command handlers ---

func handleLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	deferResponse(s, i)

	table, ok := renderLeaderboard(s)
	if !ok {
		followupEmbed(s, i, infoEmbed("Wordle Leaderboard", "No results recorded yet — check back after the next daily summary."))
		return
	}

	embed := infoEmbed("🟩 Wordle Leaderboard", fmt.Sprintf("```\n%s```", table))
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: fmt.Sprintf("%d days tracked · Pts: 1/6=6, 2/6=5 … 6/6=1, X=0 · Crwn = daily wins", board.Days()),
	}
	followupEmbed(s, i, embed)
}

// renderLeaderboard builds the aligned standings table. The second return value
// is false when there are no results yet.
func renderLeaderboard(s *discordgo.Session) (string, bool) {
	ranked := board.Ranked()
	if len(ranked) == 0 {
		return "", false
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-2s %-14s %4s %5s %5s %5s %4s\n", "#", "Player", "Pts", "Avg", "Crwn", "Win%", "Pld")
	for idx, p := range ranked {
		name := resolveName(s, p.userID)
		if len(name) > 14 {
			name = name[:14]
		}
		fmt.Fprintf(&b, "%-2d %-14s %4d %5.2f %5d %4.0f%% %4d\n",
			idx+1, name, p.points, p.avg(), p.crowns, p.winPct(), p.played)
	}
	return b.String(), true
}

func handleRefresh(s *discordgo.Session, i *discordgo.InteractionCreate) {
	deferResponse(s, i)
	if err := board.Refresh(s, channelID); err != nil {
		followupEmbed(s, i, errorEmbed("Refresh Failed", err.Error()))
		return
	}
	followupEmbed(s, i, successEmbed("Refreshed",
		fmt.Sprintf("Re-scanned #wordle: %d days tracked across %d players.", board.Days(), board.Players())))
}

// nameCache memoizes resolved display names to avoid hammering the REST API.
var nameCache = map[string]string{}

// resolveName returns a player's server nickname, global name, or username.
func resolveName(s *discordgo.Session, userID string) string {
	if n, ok := nameCache[userID]; ok {
		return n
	}
	name := userID
	if guildID != "" {
		if m, err := s.GuildMember(guildID, userID); err == nil {
			switch {
			case m.Nick != "":
				name = m.Nick
			case m.User != nil && m.User.GlobalName != "":
				name = m.User.GlobalName
			case m.User != nil:
				name = m.User.Username
			}
		}
	}
	if name == userID {
		if u, err := s.User(userID); err == nil {
			if u.GlobalName != "" {
				name = u.GlobalName
			} else {
				name = u.Username
			}
		}
	}
	nameCache[userID] = name
	return name
}
