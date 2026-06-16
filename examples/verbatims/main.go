// Verbatims example — a tiny TUI walkthrough that shows the
// border-implies-wrap contract on verbatim, result, and output
// blocks.
//
// Same demo, three border modes. Flip with --border to see how the
// TUI renderer handles long content (curl payloads, JSON blobs,
// captured stdout):
//
//	go run ./examples/verbatims/                       # default: horizontal
//	go run ./examples/verbatims/ --border full         # framed + soft-wrapped
//	go run ./examples/verbatims/ --border horizontal   # raw between top/bottom rules
//	go run ./examples/verbatims/ --border none         # raw, no rules, flush left
//
// The contrast worth noticing: with --border full, the long curl
// JSON gets soft-wrapped mid-string and mouse-select picks up the
// inserted line breaks — copy-paste round-trip is broken. With
// --border horizontal or --border none, the long line is emitted
// raw and round-trips cleanly. Pre-wrap with \n if you want
// explicit line breaks in raw mode.
//
// Single-block per-step contrast (one verbatim wrapped, one raw, in
// the same demo run) is tracked in issue 66.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/tui"
)

func main() {
	border := parseBorderFlag(os.Args[1:])

	demo := demokit.New("Verbatim border modes").
		Description(fmt.Sprintf("Long curl payloads under --border %s — copy-paste check.", borderName(border))).
		Dir("verbatims").
		MaxSteps(10).
		MaxVisits(1).
		BoxedVerbatim()

	demo.Section("How to read this demo",
		"Each step has a verbatim block with a long line — curl + JSON, or a piped jq.",
		"With --border full the line wraps inside the box; mouse-select picks up the wrap.",
		"With --border horizontal (default) or none, the line stays raw and pastes cleanly.",
		"Re-run with --border full to compare.",
	)

	demo.Step("Mint a session").ID("mint").
		Note("The MCP handshake mints a session id. The curl payload is one long line — exactly the kind that breaks mid-string when soft-wrapped.").
		Verbatim("Run this",
			`SID=$(curl -s -X POST http://localhost:8080/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":"i","method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"skills-host","version":"1.0"},"capabilities":{}}}' -D - -o /dev/null | grep -i 'mcp-session-id' | awk '{print $2}' | tr -d '\r\n')`).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("SID=abc123def456ghi789jkl012mno345pqr678stu901vwx234yz")
			return nil
		})

	demo.Step("Probe a stateless server").ID("probe").
		Note("Two ways to do the same probe. The result box below shows captured stdout — same wrap rules apply there.").
		VerbatimVariants("server/discover",
			demokit.MakeVariant("curl", "bash",
				`curl -s -X POST http://localhost:8080/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":"d","method":"server/discover","params":{}}' | jq '.result'`).Default(),
			demokit.MakeVariant("httpie", "bash",
				`http POST http://localhost:8080/mcp Content-Type:application/json Accept:'application/json, text/event-stream' jsonrpc=2.0 id=d method=server/discover params:='{}' | jq '.result'`),
		).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println(`{"name":"demokit-mcp-host","version":"0.1.0","capabilities":{"tools":{"listChanged":true},"prompts":{"listChanged":true},"resources":{"subscribe":true,"listChanged":true}},"protocolVersion":"2025-11-25","instructions":"Long stdout lines route through the same border-implies-wrap contract."}`)
			return demokit.Info("captured stdout flows through the result box — same contract")
		})

	demo.Step("Recap").ID("recap").
		Note("Re-run with a different --border to compare. The verbatim blocks above and the result block in step 2 all share the same contract: side border = framed + wrapped, no side border = raw.").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			return nil
		})

	r := tui.New().WithBorderStyle(border)
	demo.WithRenderer(r)
	demo.Execute()
}

func parseBorderFlag(args []string) demokit.BorderStyle {
	for i, a := range args {
		switch {
		case a == "--border" && i+1 < len(args):
			return borderFromName(args[i+1])
		case strings.HasPrefix(a, "--border="):
			return borderFromName(strings.TrimPrefix(a, "--border="))
		}
	}
	return demokit.BorderHorizontalOnly
}

func borderFromName(s string) demokit.BorderStyle {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "full":
		return demokit.BorderFull
	case "none":
		return demokit.BorderNone
	default:
		return demokit.BorderHorizontalOnly
	}
}

func borderName(b demokit.BorderStyle) string {
	switch b {
	case demokit.BorderFull:
		return "full"
	case demokit.BorderNone:
		return "none"
	default:
		return "horizontal"
	}
}
