# xn-wordle

Wordle leaderboard bot for XAN NATION's `#wordle` channel.

Discord's official Wordle app posts a daily summary into the channel:

```
**Your group is on a 93 day streak!** 🔥🔥🔥 Here are yesterday's results:
👑 4/6: @alice @bob
5/6: @carol
6/6: @dave
X/6: @erin        ← did not solve
```

This bot reads that history, tallies each player, and exposes the standings as a
slash command. The channel **is** the database, so there's nothing to persist:
on startup it backfills the full history, then re-scans every 15 minutes.

## Scoring

| Result | Points |
|--------|--------|
| 1/6 | 6 |
| 2/6 | 5 |
| 3/6 | 4 |
| 4/6 | 3 |
| 5/6 | 2 |
| 6/6 | 1 |
| X/6 | 0 |

An **FF** ("failure to finish") — a player started the puzzle but never finished
it — is scored exactly like an `X/6`: a failed puzzle worth 0 points.

A **hole in one** (⛳) is a first-guess solve — the puzzle cracked on guess 1.
It's tracked as a permanent per-player counter and shown on the leaderboard.

`/leaderboard` ranks by total **Pts** (tie-break: lowest average guesses, then
most crowns, then most holes-in-one) and shows points, average guesses, 👑 crowns
(daily wins), ⛳ holes-in-one, FF, win %, and days played.

## Corrections

The channel is the database, but the Wordle app sometimes renders finishers as
plain text and a mis-attribution can get baked into history — a re-scan alone
can't undo it. `adjustments.json` is a small, committed ledger of hand-authored
corrections (each with a date, word, and reason). It's embedded into the binary
and folded on top of every scan as additive, clamped-at-zero deltas, so a fix
persists across rescans, stays reversible (delete the entry), and is auditable.
It currently restores the **TRACE** hole-in-one to its rightful owner.

## Commands

- `/leaderboard` — show the standings
- `/wordlerefresh` — force an immediate re-scan (admin only)

## Setup

```bash
cp .env.example .env
# fill in DISCORD_TOKEN (DISCORD_GUILD_ID / WORDLE_CHANNEL_ID default to XAN NATION)
go run .
```

## Deploy

Push a `v*` tag to build `linux/{arm64,amd64}` binaries via GitHub Releases:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

Run the binary on an always-on host under systemd:

```ini
# /etc/systemd/system/xn-wordle.service
[Unit]
Description=XAN NATION Wordle leaderboard bot
After=network-online.target

[Service]
ExecStart=/usr/local/bin/xn-wordle
Environment=ENV_FILE=/etc/xn-wordle/.env
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now xn-wordle
```

## Permissions

- **Read Message History** in `#wordle` — required.
- **`applications.commands` scope** — required for the slash commands to appear.
  Re-invite the bot with this scope if `/leaderboard` doesn't show up.
- **Message Content Intent — not needed.** The bot reads history through the REST
  API (`GET /channels/{id}/messages`), which returns message content for any bot
  with Read Message History. The privileged Message Content Intent only gates the
  real-time gateway stream, which we don't use — history is polled every 15
  minutes instead. (Switching to live, event-driven updates *would* require that
  intent plus an `on_message` handler.)
