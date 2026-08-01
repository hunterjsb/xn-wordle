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
	got := parseResults(sample93, nil)
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
	got := parseResults(sample92, nil)
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
		if got := parseResults(content, nil); got != nil {
			t.Errorf("non-results message parsed to %v", got)
		}
	}
}

func TestParseResults_PlainTextNames(t *testing.T) {
	// The app sometimes posts plain "@name" finishers instead of <@id> mentions.
	const content = "**Your group is on an 80 day streak!** 🔥 Here are yesterday's results:\n" +
		"👑 3/6: @samboyd\n6/6: @pogoyim @temple"
	name2id := map[string]string{"samboyd": "111", "pogoyim": "222", "temple": "333"}

	// Without a name map, plain-text finishers are unresolvable.
	if got := parseResults(content, nil); got != nil {
		t.Errorf("expected nil without name map, got %v", got)
	}
	// With the map, all three resolve.
	got := parseResults(content, name2id)
	if len(got) != 3 {
		t.Fatalf("expected 3 finishers, got %d (%v)", len(got), got)
	}
	if e := got["111"]; e.guesses != 3 || !e.crown {
		t.Errorf("samboyd: want 3/crown, got %d/%v", e.guesses, e.crown)
	}
	if e := got["333"]; e.guesses != 6 || e.crown {
		t.Errorf("temple: want 6/no-crown, got %d/%v", e.guesses, e.crown)
	}
}

func TestParseResults_MultiWordPlainName(t *testing.T) {
	// A single-token regex would capture only "david" from "@david wolfze" and
	// leave the rest — and thus the whole name — unresolved even though the
	// full two-word alias is known.
	const content = "**Your group is on a 130 day streak!** 🔥🔥🔥 Here are yesterday's results:\n" +
		"👑 4/6: @bones? <@166272880462528513> @david wolfze\n" +
		"5/6: @pogoyim <@1054567024422047804>"
	name2id := map[string]string{
		"david wolfze": "226858912777764866",
		"bones?":       "300110170858717184",
		"pogoyim":      "213709745964580874",
	}

	got := parseResults(content, name2id)
	if len(got) != 5 {
		t.Fatalf("expected 5 finishers, got %d (%v)", len(got), got)
	}
	if e := got["226858912777764866"]; e.guesses != 4 || !e.crown {
		t.Errorf("david wolfze: want 4/crown, got %d/%v", e.guesses, e.crown)
	}
	if e := got["300110170858717184"]; e.guesses != 4 || !e.crown {
		t.Errorf("bones?: want 4/crown, got %d/%v", e.guesses, e.crown)
	}
	if e := got["213709745964580874"]; e.guesses != 5 || e.crown {
		t.Errorf("pogoyim: want 5/no-crown, got %d/%v", e.guesses, e.crown)
	}
}

func TestParsePlaying(t *testing.T) {
	cases := []struct {
		content   string
		want      []string
		wantOther bool
	}{
		{"temple was playing", []string{"temple"}, false},
		{"david wolfze and samboyd were playing", []string{"david wolfze", "samboyd"}, false},
		{"tft? is playing", []string{"tft?"}, false},
		{"temple are playing", []string{"temple"}, false},
		{"pogoyim and 2 others were playing", []string{"pogoyim"}, true},
		{"a, b and c were playing", []string{"a", "b", "c"}, false},
	}
	for _, c := range cases {
		got, other := parsePlaying(c.content)
		if other != c.wantOther {
			t.Errorf("%q: hadOthers=%v want %v", c.content, other, c.wantOther)
		}
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v want %v", c.content, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: got %v want %v", c.content, got, c.want)
				break
			}
		}
	}
}

func TestIsPlaying(t *testing.T) {
	if !isPlaying("temple was playing") {
		t.Error("expected playing notice to be detected")
	}
	if isPlaying(sample93) {
		t.Error("results summary must not be treated as a playing notice")
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
