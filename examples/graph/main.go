// Graph example — a choose-your-own-adventure auth-failure walkthrough.
//
// Demonstrates demokit's state-machine features:
//   - Steps with IDs and StepResult.Next routing (forward and backward jumps)
//   - Declarative inputs with Choice() and sticky-on-retry defaults
//   - AutoAcceptAfter countdown for time-based advancement
//   - Recording (--record path.json) and replay (--replay path.json)
//   - Trace-driven docs (--doc md --from path.json, --doc html --from path.json)
//
// Try:
//
//	go run ./examples/graph/                                    # interactive
//	go run ./examples/graph/ --tui                              # TUI box style
//	go run ./examples/graph/ --record /tmp/run.json             # save a trace
//	go run ./examples/graph/ --replay /tmp/run.json             # replay it
//	go run ./examples/graph/ --doc md --from /tmp/run.json      # markdown doc
//	go run ./examples/graph/ --doc html --from /tmp/run.json    # html doc
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/tui"
	"github.com/panyam/demokit/web"
)

type Diagnosis struct {
	Symptom string
	Retry   string
}

func main() {
	demo := demokit.New("Auth Failure Triage").
		Description("Pick a symptom and walk the recovery path").
		Dir("graph").
		MaxSteps(20).
		MaxVisits(3).
		AutoAcceptAfter(8 * time.Second).
		ShowCountdown(true).
		Actors(
			demokit.Actor("User", "User"),
			demokit.Actor("App", "App"),
			demokit.Actor("AS", "Auth Server"),
		)

	// Enable --doc bundle and --serve.
	web.RegisterWith(demo)

	demo.Section("How this demo works",
		"You'll be asked to pick a failure symptom. Each branch shows the",
		"recovery flow for that case. At the end you can loop back and",
		"try a different one.",
	)

	demo.Step("Pick a symptom").ID("triage").
		Note(
			"Most auth failures fall into a handful of buckets:",
			"",
			"- **expired** — the access token's TTL has elapsed",
			"- **scope** — token is valid but lacks the required scope",
			"- **ratelimit** — auth server is shedding load",
		).
		Input(demokit.Choice("expired", "scope", "ratelimit").
			Named("symptom", "Symptom (expired/scope/ratelimit)").
			WithDefault("expired")).
		Coalesce(func(m map[string]any) any {
			return Diagnosis{Symptom: m["symptom"].(string)}
		}).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			d := ctx.Input.(Diagnosis)
			fmt.Printf("Investigating: %s\n", d.Symptom)
			switch d.Symptom {
			case "expired":
				return &demokit.StepResult{Next: "expired"}
			case "scope":
				return &demokit.StepResult{Next: "scope"}
			case "ratelimit":
				return &demokit.StepResult{Next: "ratelimit"}
			}
			return demokit.Errf("unknown symptom: %s", d.Symptom)
		})

	// --- expired branch ---

	demo.Step("Expired token").ID("expired").
		Arrow("App", "AS", "GET /api (Bearer expired)").
		DashedArrow("AS", "App", "401 token_expired").
		Note("The access token's TTL has elapsed; the API rejects it.").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("API said: 401 token_expired")
			return demokit.Errf("token expired")
		})

	demo.Step("Refresh now?").ID("ask-refresh").
		Input(demokit.Choice("yes", "no").
			Named("retry", "Refresh? (yes/no)").
			WithDefault("yes")).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			if ctx.Inputs["retry"] == "yes" {
				return &demokit.StepResult{Next: "refresh"}
			}
			return &demokit.StepResult{Next: "abandon"}
		})

	demo.Step("Refresh succeeds").ID("refresh").
		Arrow("App", "AS", "POST /token (refresh)").
		DashedArrow("AS", "App", "{access_token, expires_in: 3600}").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("New token: eyJhbGci...truncated")
			return &demokit.StepResult{Next: "recovered"}
		})

	// --- scope branch ---

	demo.Step("Insufficient scope").ID("scope").
		Arrow("App", "AS", "GET /admin/users").
		DashedArrow("AS", "App", "403 insufficient_scope").
		Note("The token is valid but lacks the required scope.").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("AS said: 403 insufficient_scope (need admin:read)")
			return demokit.Errf("missing scope")
		})

	demo.Step("Request new scope?").ID("ask-scope").
		Input(demokit.Choice("yes", "no").
			Named("ask", "Request? (yes/no)").
			WithDefault("yes")).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			if ctx.Inputs["ask"] == "yes" {
				return &demokit.StepResult{Next: "consent"}
			}
			return &demokit.StepResult{Next: "abandon"}
		})

	demo.Step("Operator grants scope").ID("consent").
		Arrow("App", "AS", "GET /authorize?scope=admin:read").
		DashedArrow("AS", "App", "consent screen → granted").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("Operator approved admin:read")
			return &demokit.StepResult{Next: "recovered"}
		})

	// --- rate-limit branch ---

	demo.Step("Rate-limited").ID("ratelimit").
		Arrow("App", "AS", "POST /token").
		DashedArrow("AS", "App", "429 Too Many Requests (Retry-After: 5)").
		Note("Auth server is shedding load; back off and retry.").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("Backing off 5s, then retrying once...")
			return demokit.Warn("rate limited")
		})

	demo.Step("Retry succeeds").ID("ratelimit-retry").
		Arrow("App", "AS", "POST /token (after backoff)").
		DashedArrow("AS", "App", "{access_token}").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("Token issued on retry.")
			return &demokit.StepResult{Next: "recovered"}
		})

	// --- terminal nodes ---

	demo.Step("Recovered").ID("recovered").
		Note("Application has a usable token again.").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("Resumed normal operation.")
			return &demokit.StepResult{Next: "loop"}
		})

	demo.Step("Try another symptom?").ID("loop").
		Input(demokit.Choice("yes", "no").
			Named("again", "Loop? (yes/no)").
			WithDefault("no")).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			if ctx.Inputs["again"] == "yes" {
				return &demokit.StepResult{Next: "triage"}
			}
			return nil // fall through to End
		})

	demo.Step("Abandoned").ID("abandon").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			return demokit.Info("Skipped recovery; user gives up.")
		})

	demo.Step("End").ID("end")

	// --- renderer / output mode flags ---

	useTUI := false
	for _, arg := range os.Args[1:] {
		switch strings.TrimSpace(arg) {
		case "--tui":
			useTUI = true
		}
	}
	if useTUI {
		demo.WithRenderer(tui.New())
	}

	demo.Execute()
}
