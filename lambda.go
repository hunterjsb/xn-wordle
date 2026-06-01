//go:build lambda

// Command xn-wordle (lambda build) runs as an AWS Lambda invoked on an EventBridge
// schedule: it scans #wordle, builds the leaderboard, and posts it to a channel via
// REST — no gateway connection, no privileged intent. Shared logic lives in
// config.go / wordle.go / commands.go / post.go; the gateway build (main.go) is
// excluded from this build.
package main

import (
	"context"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handler)
}

// handler runs once per scheduled invocation; the event payload is unused since
// the schedule itself is the only trigger.
func handler(ctx context.Context) error {
	return runScheduledPost()
}
