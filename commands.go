package main

import (
	"fmt"
	"strings"
	"sync"
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

	ranked := board.Ranked()
	if len(ranked) == 0 {
		followupEmbed(s, i, infoEmbed("Wordle Leaderboard", "No results recorded yet — check back after the next daily summary."))
		return
	}

	// Components V2: a green-accented container with one avatar "card" per player.
	// Primary stat (points) is emphasized; the rest is rendered as subtext (-#) to
	// keep each row calm instead of a dense bolded line.
	accent := colorInfo
	medals := []string{"🥇", "🥈", "🥉"}
	container := discordgo.Container{
		AccentColor: &accent,
		Components: []discordgo.MessageComponent{
			discordgo.TextDisplay{Content: "## 🟩 Wordle Leaderboard"},
			discordgo.Separator{},
		},
	}
	for idx, p := range ranked {
		rank := fmt.Sprintf("#%d", idx+1)
		if idx < len(medals) {
			rank = medals[idx]
		}
		text := fmt.Sprintf("**%s %s** — %d pts\n-# %.2f avg · %d 👑 · %d FF · %.0f%% won · %d played",
			rank, resolveName(s, p.userID), p.points, p.avg(), p.crowns, p.ff, p.winPct(), p.played)
		section := discordgo.Section{
			Components: []discordgo.MessageComponent{discordgo.TextDisplay{Content: text}},
		}
		if url := resolveAvatar(s, p.userID); url != "" {
			section.Accessory = discordgo.Thumbnail{Media: discordgo.UnfurledMediaItem{URL: url}}
		}
		container.Components = append(container.Components, section)
	}
	container.Components = append(container.Components,
		discordgo.Separator{},
		discordgo.TextDisplay{Content: fmt.Sprintf(
			"-# %d days · Pts 1/6=6 … 6/6=1, X=0 · 👑 daily wins · FF = started but didn't finish",
			board.Days())},
	)

	if _, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Flags:      discordgo.MessageFlagsIsComponentsV2,
		Components: []discordgo.MessageComponent{container},
	}); err != nil {
		followupEmbed(s, i, errorEmbed("Leaderboard", err.Error()))
	}
}

// renderLeaderboard builds the aligned standings table. The second return value
// is false when there are no results yet.
func renderLeaderboard(s *discordgo.Session) (string, bool) {
	ranked := board.Ranked()
	if len(ranked) == 0 {
		return "", false
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-2s %-14s %4s %5s %5s %4s %5s %4s\n", "#", "Player", "Pts", "Avg", "Crwn", "FF", "Win%", "Pld")
	for idx, p := range ranked {
		name := resolveName(s, p.userID)
		if len(name) > 14 {
			name = name[:14]
		}
		fmt.Fprintf(&b, "%-2d %-14s %4d %5.2f %5d %4d %4.0f%% %4d\n",
			idx+1, name, p.points, p.avg(), p.crowns, p.ff, p.winPct(), p.played)
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

// Resolved display names and avatars are memoized to avoid hammering the REST API
// on every render. The caches are cleared on each Refresh (see clearResolveCaches)
// so nickname/avatar changes show up within a refresh interval, while history stays
// keyed by the immutable user ID. The mutex guards against the refresh goroutine
// clearing the maps while a /leaderboard handler reads them.
var (
	cacheMu     sync.Mutex
	nameCache   = map[string]string{}
	avatarCache = map[string]string{}
)

// clearResolveCaches drops memoized names/avatars so they re-resolve with fresh data.
func clearResolveCaches() {
	cacheMu.Lock()
	nameCache = map[string]string{}
	avatarCache = map[string]string{}
	cacheMu.Unlock()
}

// resolveName returns a player's current server nickname, global name, or username.
func resolveName(s *discordgo.Session, userID string) string {
	cacheMu.Lock()
	n, ok := nameCache[userID]
	cacheMu.Unlock()
	if ok {
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

	cacheMu.Lock()
	nameCache[userID] = name
	cacheMu.Unlock()
	return name
}

// resolveAvatar returns a player's avatar URL (empty string if it can't be fetched).
func resolveAvatar(s *discordgo.Session, userID string) string {
	cacheMu.Lock()
	a, ok := avatarCache[userID]
	cacheMu.Unlock()
	if ok {
		return a
	}

	url := ""
	if u, err := s.User(userID); err == nil {
		url = u.AvatarURL("128")
	}

	cacheMu.Lock()
	avatarCache[userID] = url
	cacheMu.Unlock()
	return url
}
