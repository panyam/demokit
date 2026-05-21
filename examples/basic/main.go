// Basic example — a tiny choose-your-own-adventure that takes you
// through demokit's core features in under a dozen steps:
//
//   - Branching via StepResult.Next (graph routing)
//   - Declarative inputs with Choice (with sticky-on-retry defaults)
//   - Sections for narration
//   - Sequence-diagram arrows between named actors
//   - Recording, replay, and trace-driven docs (--record / --replay /
//     --doc md --from)
//
// Run modes:
//
//	go run ./examples/basic/                              # interactive
//	go run ./examples/basic/ --mode=tui                   # styled boxes (also --tui)
//	go run ./examples/basic/ --mode=notebook              # Bubble Tea notebook UI
//	go run ./examples/basic/ --smooth                     # plain w/ smooth scroll
//	go run ./examples/basic/ --non-interactive            # default-input run
//	go run ./examples/basic/ --record /tmp/r.json
//	go run ./examples/basic/ --doc md --from /tmp/r.json
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/notebookbridge"
	"github.com/panyam/demokit/tui"
)

func main() {
	demo := demokit.New("Office Coffee Crisis").
		Description("A short choose-your-own-adventure about the morning caffeine ritual").
		Dir("basic").
		MaxSteps(30).
		MaxVisits(3).
		Actors(
			demokit.Actor("You", "You"),
			demokit.Actor("Machine", "Coffee Machine"),
			demokit.Actor("Karen", "Karen from Accounting"),
		)

	demo.Section("Setting the scene",
		"It is 9:01 AM. You did not sleep enough. The standup is in 14 minutes.",
		"You approach the office coffee machine with the focus of a samurai.",
	)

	demo.Step("Approach the machine").ID("approach").
		Arrow("You", "Machine", "shuffle forward").
		DashedArrow("Machine", "You", "[hum of disappointment]").
		Note("The machine has three buttons. None of them are labelled in any way that inspires confidence.").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("The machine waits.")
			return nil
		})

	demo.Step("Pick a button").ID("choose").
		Note("Black is reliable. Sugar is suspicious. Wild Card is, allegedly, what Karen drinks.").
		Input(demokit.Choice("black", "sugar", "wild").
			Named("button", "Which button? (black/sugar/wild)").
			WithDefault("black")).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			switch ctx.Inputs["button"] {
			case "black":
				return &demokit.StepResult{Next: "black"}
			case "sugar":
				return &demokit.StepResult{Next: "sugar"}
			case "wild":
				return &demokit.StepResult{Next: "wild"}
			}
			return demokit.Errf("the machine does not recognize that button")
		})

	demo.Step("Black coffee").ID("black").
		Arrow("Machine", "You", "pours scalding hot bitter liquid").
		Note("Dependable. Functional. Tastes like burnt cardboard.").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("You drink. You feel like a productive member of society.")
			return &demokit.StepResult{Next: "again"}
		})

	demo.Step("Sugar overload").ID("sugar").
		Arrow("Machine", "You", "syrupy sludge with whipped foam").
		Note("Tastes great for 90 seconds. The crash will be biblical.").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("Energy spike achieved. You write 47 lines of code in 3 minutes.")
			fmt.Println("(They will all be reverted in code review.)")
			return demokit.Warn("a crash is inbound")
		})

	demo.Step("Wild card").ID("wild").
		Arrow("Machine", "You", "??? (smells faintly of cilantro)").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("The cup contains something purple.")
			return nil // fall through to ask
		})

	demo.Step("Drink the wild card?").ID("ask-wild").
		Arrow("Karen", "You", "raises an eyebrow from across the kitchen").
		Input(demokit.Choice("yes", "no").
			Named("brave", "Drink it? (yes/no)").
			WithDefault("no")).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			if ctx.Inputs["brave"] == "yes" {
				return &demokit.StepResult{Next: "transformed"}
			}
			return &demokit.StepResult{Next: "dignified"}
		})

	demo.Step("Transformed").ID("transformed").
		Note("You are, briefly, a cat. Karen does not seem surprised.").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("You attend standup as a cat. Nobody comments.")
			return demokit.Info("strangely productive day ahead")
		})

	demo.Step("Dignified retreat").ID("dignified").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("You pour the wild card into the plant. The plant trembles.")
			return nil
		})

	demo.Step("Try a different button?").ID("again").
		Input(demokit.Choice("yes", "no").
			Named("loop", "Loop? (yes/no)").
			WithDefault("no")).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			if ctx.Inputs["loop"] == "yes" {
				return &demokit.StepResult{Next: "choose"}
			}
			return nil // fall through to End
		})

	demo.Step("End").ID("end").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("You walk to the meeting. The day continues.")
			return nil
		})

	// --- renderer / output mode flags ---
	// --mode=plain (default) | tui | notebook. --tui is honored as a
	// deprecated alias for --mode=tui. --smooth adds a per-line
	// delay to PlainRenderer (only relevant when mode is plain).

	smooth := false
	for _, arg := range os.Args[1:] {
		if strings.TrimSpace(arg) == "--smooth" {
			smooth = true
		}
	}
	switch demokit.Mode() {
	case "tui":
		demo.WithRenderer(tui.New())
	case "notebook":
		demo.WithRenderer(notebookbridge.New())
	default:
		if smooth {
			demo.WithRenderer(&demokit.PlainRenderer{Delay: 18 * time.Millisecond})
		}
	}

	demo.Execute()
}
