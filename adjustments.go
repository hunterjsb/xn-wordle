package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
)

// adjustmentsJSON is the committed, hand-authored correction ledger, embedded so
// it ships inside both the gateway binary and the Lambda bootstrap regardless of
// working directory.
//
//go:embed adjustments.json
var adjustmentsJSON []byte

// statAdjustment is one auditable correction folded on top of the scanned tally.
// The #wordle channel is the source of truth, but the app sometimes renders a
// finisher as plain text and the pre-fix alias resolver (PR #1) could hand that
// finish to the wrong player — a mis-credit that is baked into channel history and
// therefore survives a re-scan. Adjustments are a thin override layer: additive
// deltas re-applied after every scan, so a correction (or a dock) is permanent
// rather than recomputed away, and stays fully reversible (delete the entry) and
// auditable (date / word / reason travel with it).
type statAdjustment struct {
	Date   string      `json:"date"`           // when the puzzle ran (YYYY-MM-DD)
	Word   string      `json:"word,omitempty"` // the day's answer, for humans
	Reason string      `json:"reason"`         // why this correction exists
	Deltas []statDelta `json:"deltas"`         // per-player stat changes
}

// statDelta is a signed change to one player's surfaced stats. Positive values
// restore/credit; negative values dock. A delta with an empty user_id is a
// documented placeholder (e.g. a dock target awaiting confirmation) and is skipped.
type statDelta struct {
	UserID     string `json:"user_id"`
	Note       string `json:"note,omitempty"`
	Points     int    `json:"points,omitempty"`
	HolesInOne int    `json:"holes_in_one,omitempty"`
	Wins       int    `json:"wins,omitempty"`
	Crowns     int    `json:"crowns,omitempty"`
	FF         int    `json:"ff,omitempty"`
	Played     int    `json:"played,omitempty"`
}

// loadAdjustments parses the embedded correction ledger.
func loadAdjustments() ([]statAdjustment, error) {
	if len(adjustmentsJSON) == 0 {
		return nil, nil
	}
	var adj []statAdjustment
	if err := json.Unmarshal(adjustmentsJSON, &adj); err != nil {
		return nil, fmt.Errorf("parse adjustments.json: %w", err)
	}
	return adj, nil
}

// applyAdjustments folds the corrections into a freshly-scanned tally. Deltas are
// additive; every counter is clamped at zero so a dock can never drive a stat
// negative. Deltas with an empty user_id are skipped so a placeholder can't spawn
// a phantom player.
func applyAdjustments(stats map[string]*playerStats, adj []statAdjustment) {
	for _, a := range adj {
		for _, d := range a.Deltas {
			if d.UserID == "" {
				continue
			}
			ps := stats[d.UserID]
			if ps == nil {
				ps = &playerStats{userID: d.UserID}
				stats[d.UserID] = ps
			}
			ps.points = clampZero(ps.points + d.Points)
			ps.holesInOne = clampZero(ps.holesInOne + d.HolesInOne)
			ps.wins = clampZero(ps.wins + d.Wins)
			ps.crowns = clampZero(ps.crowns + d.Crowns)
			ps.ff = clampZero(ps.ff + d.FF)
			ps.played = clampZero(ps.played + d.Played)
		}
	}
}

// applyCommittedAdjustments loads the embedded ledger and folds it into stats. A
// malformed ledger is logged and skipped rather than failing the whole scan — a
// bad correction should never take the leaderboard offline.
func applyCommittedAdjustments(stats map[string]*playerStats) {
	adj, err := loadAdjustments()
	if err != nil {
		log.Printf("wordle: %v", err)
		return
	}
	applyAdjustments(stats, adj)
}

func clampZero(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
