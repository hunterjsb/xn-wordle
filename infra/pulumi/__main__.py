"""xn-wordle AWS infrastructure — us-east-1.

A single Lambda (provided.al2023, arm64) invoked by an EventBridge schedule to
post the daily Wordle leaderboard. Pulumi owns configuration; CI owns the binary
(see ../../.github/workflows/deploy-lambda.yml + github_oidc.py).
"""

import pulumi

from lambda_post import lambda_function, log_group, schedule_rule
from github_oidc import ci_role, oidc_provider

pulumi.export("lambda_arn", lambda_function.arn)
pulumi.export("lambda_name", lambda_function.name)
pulumi.export("log_group", log_group.name)
pulumi.export("schedule_rule", schedule_rule.name)
pulumi.export("github_oidc_provider_arn", oidc_provider.arn)
pulumi.export("ci_deploy_role_arn", ci_role.arn)
