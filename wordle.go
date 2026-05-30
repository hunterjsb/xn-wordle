package main

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// resultsMarker identifies the Wordle app's daily summary message. The app posts
// a line like "...🔥 Here are yesterday's results:" followed by one line per score.
const resultsMarker = "Here are yesterday's results"

// crown marks the line containing the day's best score(s).
const crown = "\U0001F451" // 👑

// failGuesses is the guess count assigned to an X/6 (did-not-solve) entry. It is
// 7 so it sorts behind a 6/6 and yields 0 points (7 - 7).
const failGuesses = 7

var (
	// lineRe matches a single result line, e.g. "👑 4/6: <@1> <@2>" or "X/6: <@3>".
	lineRe = regexp.MustCompile(`([1-6Xx])\s*/\s*6\s*:\s*(.*)`)
	// mentionRe pulls user IDs out of <@id> / <@!id> mentions.
	mentionRe = regexp.MustCompile(`<@!?(\d+)>`)
)

// dayEntry is one player's outcome on a single day.
type dayEntry struct {
	guesses int  // 1..6, or failGuesses for an X/6
	crown   bool // got the 👑 (best score that day)
}

// parseResults parses a Wordle app summary message into a map of userID -> outcome.
// Returns nil if the message is not a results summary or has no parseable entries.
func parseResults(content string) map[string]dayEntry {
	if !strings.Contains(content, resultsMarker) {
		return nil
	}
	entries := map[string]dayEntry{}
	for _, line := range strings.Split(content, "\n") {
		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		score := strings.ToUpper(m[1])
		guesses := failGuesses
		if score != "X" {
			guesses = int(score[0] - '0')
		}
		hasCrown := strings.Contains(line, crown)
		for _, um := range mentionRe.FindAllStringSubmatch(m[2], -1) {
			entries[um[1]] = dayEntry{guesses: guesses, crown: hasCrown}
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

// points converts a guess count to leaderboard points: 1/6=6 … 6/6=1, X/6=0.
func points(guesses int) int {
	if p := 7 - guesses; p > 0 {
		return p
	}
	return 0
}

// playerStats is a player's accumulated record across all tracked days.
type playerStats struct {
	userID     string
	points     int
	totalGuess int // sum of guesses (X counts as failGuesses) for averaging
	wins       int // days solved (guesses <= 6)
	crowns     int // days with the 👑
	played     int // total days played
}

func (p playerStats) avg() float64 {
	if p.played == 0 {
		return 0
	}
	return float64(p.totalGuess) / float64(p.played)
}

func (p playerStats) winPct() float64 {
	if p.played == 0 {
		return 0
	}
	return float64(p.wins) / float64(p.played) * 100
}

// Leaderboard is an in-memory tally rebuilt from channel history. The #wordle
// channel is the source of truth, so no persistence is needed — a restart just
// re-scans the history.
type Leaderboard struct {
	mu    sync.RWMutex
	stats map[string]*playerStats
	days  int
}

func NewLeaderboard() *Leaderboard {
	return &Leaderboard{stats: map[string]*playerStats{}}
}

func (l *Leaderboard) Days() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.days
}

func (l *Leaderboard) Players() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.stats)
}

// Refresh re-scans the entire channel history and rebuilds the tally. It reads
// messages via the REST API, which returns message content without needing the
// privileged Message Content intent.
func (l *Leaderboard) Refresh(s *discordgo.Session, channelID string) error {
	stats := map[string]*playerStats{}
	days := 0
	beforeID := ""

	for {
		msgs, err := s.ChannelMessages(channelID, 100, beforeID, "", "")
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			break
		}
		for _, m := range msgs {
			entries := parseResults(m.Content)
			if entries == nil {
				continue
			}
			days++
			for uid, e := range entries {
				ps := stats[uid]
				if ps == nil {
					ps = &playerStats{userID: uid}
					stats[uid] = ps
				}
				ps.played++
				ps.totalGuess += e.guesses
				ps.points += points(e.guesses)
				if e.guesses <= 6 {
					ps.wins++
				}
				if e.crown {
					ps.crowns++
				}
			}
		}
		beforeID = msgs[len(msgs)-1].ID
		if len(msgs) < 100 {
			break
		}
	}

	l.mu.Lock()
	l.stats = stats
	l.days = days
	l.mu.Unlock()
	return nil
}

// Ranked returns players sorted by points (desc), then average guesses (asc).
func (l *Leaderboard) Ranked() []playerStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make([]playerStats, 0, len(l.stats))
	for _, p := range l.stats {
		out = append(out, *p)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].points != out[b].points {
			return out[a].points > out[b].points
		}
		if out[a].avg() != out[b].avg() {
			return out[a].avg() < out[b].avg()
		}
		return out[a].crowns > out[b].crowns
	})
	return out
}
