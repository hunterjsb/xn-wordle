# xn-wordle infra

Serverless deploy of the **scheduled daily leaderboard post**: an EventBridge cron
triggers a Go Lambda (`provided.al2023`, arm64) that scans `#wordle` and posts the
Components V2 leaderboard. Pulumi owns the config; CI owns the binary.

```
EventBridge (cron, UTC) ──► Lambda xn-wordle ──► posts leaderboard to #wordle
```

- `lambda_post.py` — Lambda, log group, EventBridge rule + target + permission, $1 budget
- `github_oidc.py` — GitHub OIDC provider + scoped `xn-wordle-ci-deploy` role (no static keys in CI)
- Region: `us-east-1`. State: `s3://xn-wordle-pulumi-state-350985642081`.

## First deploy (run locally, personal account)

> All AWS calls use the **personal** profile. The default profile is the Gnosis
> work account, so set this explicitly for the whole session:
>
> ```bash
> export AWS_PROFILE=personal
> ```

1. **Create the Pulumi state bucket** (one time):
   ```bash
   aws s3 mb s3://xn-wordle-pulumi-state-350985642081 --region us-east-1
   ```

2. **Build the Lambda zip** (Pulumi uploads it on first create):
   ```bash
   cd ../..                       # repo root
   GOOS=linux GOARCH=arm64 go build -tags lambda -o bootstrap .
   zip bootstrap.zip bootstrap
   cd infra/pulumi
   ```

3. **Set up Pulumi** (S3 backend + Python venv):
   ```bash
   python3 -m venv venv && ./venv/bin/pip install -r requirements.txt
   export PULUMI_CONFIG_PASSPHRASE=<choose-a-passphrase>
   pulumi login s3://xn-wordle-pulumi-state-350985642081
   pulumi stack select prod || pulumi stack init prod
   ```

4. **Set the bot token as an encrypted secret:**
   ```bash
   pulumi config set --secret xn-wordle:discordToken <bot-token>
   ```

5. **Deploy:**
   ```bash
   pulumi up
   ```

That creates the Lambda (with the current zip), the daily schedule, the OIDC role,
and the budget. After this, **code changes deploy automatically via CI** — pushing
Go changes to `master` runs `.github/workflows/deploy-lambda.yml`, which assumes the
`xn-wordle-ci-deploy` role and runs `aws lambda update-function-code`. Pulumi
ignores code drift (`ignore_changes=["code"]`), so the two never fight.

## Changing config (schedule, channel, token)

Edit `Pulumi.prod.yaml` (or `pulumi config set ...`) and re-run `pulumi up`:

- `scheduleCron` — EventBridge cron, **UTC** (e.g. `cron(0 15 * * ? *)` = 15:00 UTC).
- `leaderboardChannelId` — post target; defaults to `#wordle` when unset.
- `discordToken` — `pulumi config set --secret xn-wordle:discordToken <token>`.

## Manual test / run

The same code path runs locally via the gateway binary:
```bash
WORDLE_POST_ONCE=1 DISCORD_TOKEN=… DISCORD_GUILD_ID=… WORDLE_CHANNEL_ID=… \
  LEADERBOARD_CHANNEL_ID=<test-channel> ./xn-wordle
```
