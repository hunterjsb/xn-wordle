"""xn-wordle Lambda + daily EventBridge schedule + cost guard (us-east-1).

Build artifact: ../../bootstrap.zip
Built via: cd ../.. && GOOS=linux GOARCH=arm64 go build -tags lambda -o bootstrap . && zip bootstrap.zip bootstrap
"""

import json
import os

import pulumi
import pulumi_aws as aws

REGION = "us-east-1"

cfg = pulumi.Config()
guild_id = cfg.require("guildId")
wordle_channel_id = cfg.require("wordleChannelId")
leaderboard_channel_id = cfg.get("leaderboardChannelId") or ""
schedule_cron = cfg.get("scheduleCron") or "cron(0 15 * * ? *)"
discord_token = cfg.require_secret("discordToken")
budget_email = cfg.get("budgetEmail") or "hunterjsb@gmail.com"

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LAMBDA_ZIP = os.path.join(REPO_ROOT, "bootstrap.zip")

exec_role = aws.iam.Role(
    "xn-wordle-role",
    name="xn-wordle-role",
    assume_role_policy=json.dumps(
        {
            "Version": "2012-10-17",
            "Statement": [
                {
                    "Effect": "Allow",
                    "Principal": {"Service": "lambda.amazonaws.com"},
                    "Action": "sts:AssumeRole",
                }
            ],
        }
    ),
)

aws.iam.RolePolicyAttachment(
    "xn-wordle-logs",
    role=exec_role.name,
    policy_arn="arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
)

log_group = aws.cloudwatch.LogGroup(
    "xn-wordle-logs",
    name="/aws/lambda/xn-wordle",
    retention_in_days=14,
)

lambda_function = aws.lambda_.Function(
    "xn-wordle",
    name="xn-wordle",
    role=exec_role.arn,
    runtime="provided.al2023",
    handler="bootstrap",
    architectures=["arm64"],
    code=pulumi.FileArchive(LAMBDA_ZIP),
    memory_size=256,
    timeout=60,
    # It's a once-a-day cron; cap concurrency so a misfire can't run away on cost.
    reserved_concurrent_executions=2,
    environment=aws.lambda_.FunctionEnvironmentArgs(
        variables={
            "DISCORD_TOKEN": discord_token,
            "DISCORD_GUILD_ID": guild_id,
            "WORDLE_CHANNEL_ID": wordle_channel_id,
            "LEADERBOARD_CHANNEL_ID": leaderboard_channel_id,
        },
    ),
    # CI (deploy-lambda.yml) owns the function code via `aws lambda update-function-code`.
    # Pulumi owns configuration only; ignore "code" so `pulumi up` won't clobber a fresh
    # CI deploy with whatever zip happens to be on the local disk.
    opts=pulumi.ResourceOptions(depends_on=[log_group], ignore_changes=["code"]),
)

# --- daily schedule ---
schedule_rule = aws.cloudwatch.EventRule(
    "xn-wordle-daily",
    name="xn-wordle-daily",
    description="Trigger the daily Wordle leaderboard post",
    schedule_expression=schedule_cron,
)

aws.cloudwatch.EventTarget(
    "xn-wordle-daily-target",
    rule=schedule_rule.name,
    arn=lambda_function.arn,
)

aws.lambda_.Permission(
    "xn-wordle-allow-eventbridge",
    action="lambda:InvokeFunction",
    function=lambda_function.name,
    principal="events.amazonaws.com",
    source_arn=schedule_rule.arn,
)

# --- cost guard: $1/mo Lambda budget, email at 100% ---
aws.budgets.Budget(
    "xn-wordle-budget",
    name="xn-wordle-lambda",
    budget_type="COST",
    limit_amount="1",
    limit_unit="USD",
    time_unit="MONTHLY",
    cost_filters=[
        aws.budgets.BudgetCostFilterArgs(name="Service", values=["AWS Lambda"]),
    ],
    notifications=[
        aws.budgets.BudgetNotificationArgs(
            comparison_operator="GREATER_THAN",
            threshold=100,
            threshold_type="PERCENTAGE",
            notification_type="ACTUAL",
            subscriber_email_addresses=[budget_email],
        ),
    ],
)
