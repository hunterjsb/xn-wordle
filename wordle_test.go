package main

import "testing"

// Real samples captured from #wordle (XAN NATION).
const sample93 = "**Your group is on a 93 day streak!** 🔥🔥🔥 Here are yesterday's results:\n" +
	"👑 4/6: <@300110170858717184> <@1054567024422047804>\n" +
	"5/6: <@371034483836846090>\n" +
	"6/6: <@213709745964580874> <@226858912777764866>"

const sample92 = "**Your group is on a 92 day streak!** 🔥🔥🔥 Here are yesterday's results:\n" +
	"👑 3/6: <@300110170858717184> <@166272880462528513>\n" +
	"4/6: <@1054567024422047804>\n" +
	"5/6: <@213709745964580874> <@226858912777764866>\n" +
	"X/6: <@371034483836846090>"

func TestParseResults_Sample93(t *testing.T) {
	got := parseResults(sample93)
	if got == nil {
		t.Fatal("expected entries, got nil")
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 players, got %d", len(got))
	}
	// Crowned 4/6 winners.
	for _, uid := range []string{"300110170858717184", "1054567024422047804"} {
		e := got[uid]
		if e.guesses != 4 || !e.crown {
			t.Errorf("%s: want 4/crown, got %d/%v", uid, e.guesses, e.crown)
		}
	}
	// Non-crowned lines.
	if e := got["371034483836846090"]; e.guesses != 5 || e.crown {
		t.Errorf("5/6 line: want 5/no-crown, got %d/%v", e.guesses, e.crown)
	}
	if e := got["226858912777764866"]; e.guesses != 6 || e.crown {
		t.Errorf("6/6 line: want 6/no-crown, got %d/%v", e.guesses, e.crown)
	}
}

func TestParseResults_FailIsSeven(t *testing.T) {
	got := parseResults(sample92)
	if e := got["371034483836846090"]; e.guesses != failGuesses {
		t.Errorf("X/6 should be %d guesses, got %d", failGuesses, e.guesses)
	}
	if got["371034483836846090"].crown {
		t.Error("X/6 player should not be crowned")
	}
}

func TestParseResults_NonResultsMessage(t *testing.T) {
	for _, content := range []string{
		"david wolfze and samboyd were playing",
		"most often I start with the n word",
		"",
	} {
		if got := parseResults(content); got != nil {
			t.Errorf("non-results message parsed to %v", got)
		}
	}
}

func TestPoints(t *testing.T) {
	cases := map[int]int{1: 6, 2: 5, 3: 4, 4: 3, 5: 2, 6: 1, 7: 0}
	for guesses, want := range cases {
		if got := points(guesses); got != want {
			t.Errorf("points(%d) = %d, want %d", guesses, got, want)
		}
	}
}
