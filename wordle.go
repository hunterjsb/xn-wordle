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

// failGuesses is the guess count assigned to an X/6 (did-not-solve) entry, and
// also to an FF (started but never finished). It is 7 so it sorts behind a 6/6
// and yields 0 points (7 - 7).
const failGuesses = 7

var (
	// lineRe matches a single result line, e.g. "👑 4/6: <@1> <@2>" or "X/6: <@3>".
	lineRe = regexp.MustCompile(`([1-6Xx])\s*/\s*6\s*:\s*(.*)`)
	// mentionRe pulls user IDs out of <@id> / <@!id> mentions.
	mentionRe = regexp.MustCompile(`<@!?(\d+)>`)
	// othersRe matches the app's anonymized "2 others" / "3 others" fragments,
	// which hide the names of additional players and can't be attributed.
	othersRe = regexp.MustCompile(`^\d+\s+others?$`)
	// plainNameRe matches a plain-text "@name" token. The app occasionally renders
	// finishers as plain text instead of real <@id> mentions; those are resolved
	// through the name map.
	plainNameRe = regexp.MustCompile(`@([^\s@]+)`)
	// avatarUserIDRe pulls the user ID out of a Discord CDN avatar URL, e.g.
	// https://cdn.discordapp.com/avatars/123456789/abcdef.png.
	avatarUserIDRe = regexp.MustCompile(`/avatars/(\d+)/`)
	// leaderboardRowRe matches one player card's text in the bot's own posted
	// leaderboard, e.g. "### #3 AEGIS\n253 pts  ...", capturing the display name.
	leaderboardRowRe = regexp.MustCompile(`(?s)^### \S+ (.+?)\n\d+ pts`)
)

// playingVerbs are the trailing phrases the Wordle app uses to announce a start,
// e.g. "temple was playing", "a and b were playing", "x is playing".
var playingVerbs = []string{" were playing", " was playing", " are playing", " is playing"}

// dayEntry is one player's outcome on a single day.
type dayEntry struct {
	guesses int  // 1..6, or failGuesses for an X/6
	crown   bool // got the 👑 (best score that day)
}

// parseResults parses a Wordle app summary message into a map of userID -> outcome.
// Finishers are read from <@id> mentions; when name2id is non-nil, plain-text
// "@name" finishers (which the app sometimes posts instead of real mentions) are
// resolved through it too. Returns nil if the message is not a results summary or
// has no resolvable entries.
func parseResults(content string, name2id map[string]string) map[string]dayEntry {
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
		rest := m[2]
		for _, um := range mentionRe.FindAllStringSubmatch(rest, -1) {
			entries[um[1]] = dayEntry{guesses: guesses, crown: hasCrown}
		}
		if name2id != nil {
			// Strip real mentions first so their inner "@id" isn't re-matched.
			plain := mentionRe.ReplaceAllString(rest, "")
			for uid := range resolvePlainNames(plain, name2id) {
				entries[uid] = dayEntry{guesses: guesses, crown: hasCrown}
			}
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

// resolvePlainNames matches known aliases (name2id keys, which may be
// multi-word — e.g. "david wolfze") against the text remaining after real
// mentions are stripped. Longest alias first, by word count, so a two-word
// name matches as a whole instead of a single-token regex silently capturing
// just its first word and leaving the rest (and thus the whole name) unresolved.
func resolvePlainNames(rest string, name2id map[string]string) map[string]bool {
	tokens := strings.Fields(strings.ReplaceAll(rest, "@", " "))
	if len(tokens) == 0 {
		return nil
	}

	type alias struct {
		name  string
		words []string
		id    string
	}
	aliases := make([]alias, 0, len(name2id))
	for name, id := range name2id {
		if words := strings.Fields(name); len(words) > 0 {
			aliases = append(aliases, alias{name: name, words: words, id: id})
		}
	}
	// Longest alias first so multi-word names claim their tokens before a shorter
	// overlapping alias can. Break ties on the alias text (then id) so two
	// equal-length aliases always resolve the same way: without a total order the
	// greedy match below depends on aliases' arrival order — which is randomized
	// Go map-iteration order — making a shared-token finisher (and thus a
	// started-but-flipped player's FF) flip between rescans of identical history.
	sort.Slice(aliases, func(i, j int) bool {
		if len(aliases[i].words) != len(aliases[j].words) {
			return len(aliases[i].words) > len(aliases[j].words)
		}
		if aliases[i].name != aliases[j].name {
			return aliases[i].name < aliases[j].name
		}
		return aliases[i].id < aliases[j].id
	})

	used := make([]bool, len(tokens))
	resolved := map[string]bool{}
	for _, a := range aliases {
		n := len(a.words)
		for i := 0; i+n <= len(tokens); i++ {
			if used[i] {
				continue
			}
			match := true
			for j := 0; j < n; j++ {
				if used[i+j] || !strings.EqualFold(tokens[i+j], a.words[j]) {
					match = false
					break
				}
			}
			if match {
				resolved[a.id] = true
				for j := 0; j < n; j++ {
					used[i+j] = true
				}
				break
			}
		}
	}
	return resolved
}

// isPlaying reports whether a message is one of the app's "X was playing" notices.
func isPlaying(content string) bool {
	return strings.Contains(content, "playing") && !strings.Contains(content, resultsMarker)
}

// parsePlaying extracts the display names from a "playing" notice. The boolean is
// true when the message anonymized some players as "N others" — those starts are
// unattributable and intentionally dropped.
func parsePlaying(content string) ([]string, bool) {
	c := strings.TrimSpace(content)
	for _, v := range playingVerbs {
		if strings.HasSuffix(c, v) {
			c = c[:len(c)-len(v)]
			break
		}
	}
	c = strings.ReplaceAll(c, " and ", ",")
	var names []string
	hadOthers := false
	for _, part := range strings.Split(c, ",") {
		n := strings.TrimSpace(part)
		if n == "" {
			continue
		}
		if othersRe.MatchString(n) {
			hadOthers = true
			continue
		}
		names = append(names, n)
	}
	return names, hadOthers
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
	totalGuess int // sum of guesses (X and FF each count as failGuesses) for averaging
	wins       int // days solved (guesses <= 6)
	crowns     int // days with the 👑
	holesInOne int // days solved on the very first guess — a "hole in one"
	ff         int // days started but never finished
	played     int // total days played (finishes + FFs)
}

// creditFinisher folds one day's finish into a player's running tally. Kept as a
// standalone step (rather than inline in Refresh) so the scoring rules — including
// hole-in-one detection — are unit-testable without a live Discord session.
func creditFinisher(ps *playerStats, e dayEntry) {
	ps.played++
	ps.totalGuess += e.guesses
	ps.points += points(e.guesses)
	if e.guesses <= 6 {
		ps.wins++
	}
	// A hole in one is a first-guess solve: the day's puzzle cracked on guess 1.
	if e.guesses == 1 {
		ps.holesInOne++
	}
	if e.crown {
		ps.crowns++
	}
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

// fetchAll pages the entire channel history and returns it oldest-first. Reading
// via the REST API returns message content without the privileged Message Content
// intent.
func fetchAll(s *discordgo.Session, channelID string) ([]*discordgo.Message, error) {
	var all []*discordgo.Message
	beforeID := ""
	for {
		msgs, err := s.ChannelMessages(channelID, 100, beforeID, "", "")
		if err != nil {
			return nil, err
		}
		if len(msgs) == 0 {
			break
		}
		all = append(all, msgs...)
		beforeID = msgs[len(msgs)-1].ID
		if len(msgs) < 100 {
			break
		}
	}
	// ChannelMessages returns newest-first; reverse to chronological order.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all, nil
}

// buildNameMap maps lower-cased display names (nick / global name / username) to
// user IDs, so the names in "playing" notices can be matched to results mentions.
// It resolves known finishers plus any extra names seen only in playing notices.
func buildNameMap(s *discordgo.Session, finisherIDs, playingNames map[string]bool) map[string]string {
	name2id := map[string]string{}
	add := func(m *discordgo.Member) {
		if m == nil || m.User == nil {
			return
		}
		for _, n := range []string{m.Nick, m.User.GlobalName, m.User.Username} {
			if n != "" {
				name2id[strings.ToLower(n)] = m.User.ID
			}
		}
	}
	for uid := range finisherIDs {
		if m, err := s.GuildMember(guildID, uid); err == nil {
			add(m)
		}
	}
	for name := range playingNames {
		if _, ok := name2id[strings.ToLower(name)]; ok {
			continue
		}
		if members, err := s.GuildMembersSearch(guildID, name, 5); err == nil {
			for _, m := range members {
				add(m)
			}
		}
	}
	return name2id
}

// historicalAliases mines the bot's own past leaderboard posts for every
// display name a player has ever been shown under, keyed by user ID (read off
// the avatar thumbnail's CDN URL on each player card). A server nickname
// change makes buildNameMap forget the old name — since it's rebuilt from
// current guild state on every scan — which silently drops that player's past
// plain-text-mention credit (points/plays/crowns) on the very next rescan.
// The channel already holds this history in the bot's own prior posts, so no
// separate persistence is needed to recover it.
func historicalAliases(botID string, msgs []*discordgo.Message) map[string]string {
	aliases := map[string]string{}
	if botID == "" {
		return aliases
	}
	for _, m := range msgs {
		if m.Author == nil || m.Author.ID != botID {
			continue
		}
		for _, top := range m.Components {
			container, ok := top.(*discordgo.Container)
			if !ok {
				continue
			}
			for _, c := range container.Components {
				section, ok := c.(*discordgo.Section)
				if !ok {
					continue
				}
				var uid string
				if thumb, ok := section.Accessory.(*discordgo.Thumbnail); ok {
					if am := avatarUserIDRe.FindStringSubmatch(thumb.Media.URL); am != nil {
						uid = am[1]
					}
				}
				if uid == "" {
					continue
				}
				for _, inner := range section.Components {
					text, ok := inner.(*discordgo.TextDisplay)
					if !ok {
						continue
					}
					if rm := leaderboardRowRe.FindStringSubmatch(text.Content); rm != nil {
						aliases[strings.ToLower(rm[1])] = uid
					}
				}
			}
		}
	}
	return aliases
}

// Refresh re-scans the entire channel history and rebuilds the tally, including
// FF (started-but-never-finished) detection.
func (l *Leaderboard) Refresh(s *discordgo.Session, channelID string) error {
	msgs, err := fetchAll(s, channelID)
	if err != nil {
		return err
	}

	// First pass: collect finisher IDs (from real mentions) plus every display
	// name that appears in playing notices or as a plain-text "@name" finisher, so
	// the name map can resolve them all.
	finisherIDs := map[string]bool{}
	extraNames := map[string]bool{}
	for _, m := range msgs {
		if strings.Contains(m.Content, resultsMarker) {
			for uid := range parseResults(m.Content, nil) {
				finisherIDs[uid] = true
			}
			for _, line := range strings.Split(m.Content, "\n") {
				lm := lineRe.FindStringSubmatch(line)
				if lm == nil {
					continue
				}
				plain := mentionRe.ReplaceAllString(lm[2], "")
				for _, pm := range plainNameRe.FindAllStringSubmatch(plain, -1) {
					extraNames[pm[1]] = true
				}
			}
		} else if isPlaying(m.Content) {
			names, _ := parsePlaying(m.Content)
			for _, n := range names {
				extraNames[n] = true
			}
		}
	}
	name2id := buildNameMap(s, finisherIDs, extraNames)

	// Fill in names dropped by a nickname change with what the player was
	// called in the bot's own past posts. Current guild state wins on conflict.
	botID := ""
	if me, err := s.User("@me"); err == nil {
		botID = me.ID
	}
	for name, uid := range historicalAliases(botID, msgs) {
		if _, exists := name2id[name]; !exists {
			name2id[name] = uid
		}
	}

	// Second pass (chronological): window the "playing" notices between result
	// summaries. Each summary reports the prior puzzle, so the starts accumulated
	// since the last summary belong to the puzzle it reports. FF = started but not
	// among that puzzle's finishers.
	stats := map[string]*playerStats{}
	days := 0
	started := map[string]bool{} // user IDs seen "playing" in the current window
	get := func(uid string) *playerStats {
		ps := stats[uid]
		if ps == nil {
			ps = &playerStats{userID: uid}
			stats[uid] = ps
		}
		return ps
	}

	for _, m := range msgs {
		// A results summary marks a puzzle boundary: credit finishers, then the
		// players who started this window but aren't among the finishers are FFs.
		if strings.Contains(m.Content, resultsMarker) {
			entries := parseResults(m.Content, name2id)
			if entries != nil {
				days++
				for uid, e := range entries {
					creditFinisher(get(uid), e)
				}
				for uid := range started {
					if _, finished := entries[uid]; !finished {
						ps := get(uid)
						ps.played++
						ps.totalGuess += failGuesses
						ps.ff++
					}
				}
			}
			started = map[string]bool{}
			continue
		}
		if isPlaying(m.Content) {
			names, _ := parsePlaying(m.Content)
			for _, n := range names {
				if uid, ok := name2id[strings.ToLower(n)]; ok {
					started[uid] = true
				}
			}
		}
	}

	// Fold in the committed manual corrections. #wordle history is the source of
	// truth, but a mis-attribution baked into that history (see the TRACE hole-in-one
	// in adjustments.json) can't be undone by re-scanning — the override layer is
	// re-applied on every scan so a correction persists instead of being recomputed away.
	applyCommittedAdjustments(stats)

	l.mu.Lock()
	l.stats = stats
	l.days = days
	l.mu.Unlock()

	// Drop memoized names/avatars so renames and avatar changes are picked up.
	clearResolveCaches()
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
		if out[a].crowns != out[b].crowns {
			return out[a].crowns > out[b].crowns
		}
		// Deepest tiebreak: more first-guess solves ranks higher.
		return out[a].holesInOne > out[b].holesInOne
	})
	return out
}
