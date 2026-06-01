"""GitHub Actions OIDC trust for CI-driven Lambda code deploys.

References the account's existing GitHub OIDC provider (there's one per account —
already created by another project) and defines a single IAM role GitHub Actions
can assume to push new xn-wordle Lambda code via `aws lambda update-function-code`.
No static keys in CI. Pulumi-managed infra changes stay manual. Trust is scoped to
this repo, master branch + v* tags only.
"""

import json

import pulumi_aws as aws

REGION = "us-east-1"
REPO = "hunterjsb/xn-wordle"

account_id = aws.get_caller_identity().account_id

# The GitHub OIDC provider is account-global and already exists; reference its
# deterministic ARN rather than managing the resource here.
oidc_provider_arn = (
    f"arn:aws:iam::{account_id}:oidc-provider/token.actions.githubusercontent.com"
)

trust_policy = json.dumps(
    {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Principal": {"Federated": oidc_provider_arn},
                "Action": "sts:AssumeRoleWithWebIdentity",
                "Condition": {
                    "StringEquals": {
                        "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
                    },
                    "StringLike": {
                        "token.actions.githubusercontent.com:sub": [
                            f"repo:{REPO}:ref:refs/heads/master",
                            f"repo:{REPO}:ref:refs/tags/v*",
                        ]
                    },
                },
            }
        ],
    }
)

ci_role = aws.iam.Role(
    "xn-wordle-ci-deploy",
    name="xn-wordle-ci-deploy",
    description="Role assumed by GitHub Actions to deploy xn-wordle Lambda code",
    assume_role_policy=trust_policy,
)

# Code-only deploys: read for verify/wait, plus update + publish. No Pulumi state,
# no PassRole, no IAM/Logs writes.
aws.iam.RolePolicy(
    "xn-wordle-ci-deploy-policy",
    role=ci_role.id,
    policy=json.dumps(
        {
            "Version": "2012-10-17",
            "Statement": [
                {
                    "Sid": "LambdaCodeDeploy",
                    "Effect": "Allow",
                    "Action": [
                        "lambda:GetFunction",
                        "lambda:GetFunctionConfiguration",
                        "lambda:UpdateFunctionCode",
                        "lambda:PublishVersion",
                    ],
                    "Resource": f"arn:aws:lambda:{REGION}:{account_id}:function:xn-wordle",
                }
            ],
        }
    ),
)
