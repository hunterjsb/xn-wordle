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

This bot reads that history (via the REST API — no privileged Message Content
intent needed), tallies each player, and exposes the standings as a slash
command. The channel **is** the database, so there's nothing to persist: on
startup it backfills the full history, then re-scans every 15 minutes.

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

`/leaderboard` ranks by total **Pts** (tie-break: lowest average guesses, then
most crowns) and shows points, average guesses, 👑 crowns (daily wins), win %,
and days played.

## Commands

- `/leaderboard` — show the standings
- `/wordlerefresh` — force an immediate re-scan (admin only)

## Setup

```bash
cp .env.example .env
# fill in DISCORD_TOKEN (DISCORD_GUILD_ID / WORDLE_CHANNEL_ID default to XAN NATION)
go run .
```

The bot needs **Read Message History** in `#wordle` and the
`applications.commands` scope (re-invite the bot with that scope if the slash
commands don't appear).

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
