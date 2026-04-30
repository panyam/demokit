// Cave of Cards — a sidecar-markdown adventure that stresses the
// directed-graph machinery: multiple cycles, a converging join, and
// one piece of Go-side state (a magic ring) that decides the outcome
// at the dragon. All prose lives in demo.md; this file wires Run
// closures via Bind, threads the ring state, and starts the demo.
//
// Try:
//
//	go run ./examples/dungeon/                              # interactive
//	go run ./examples/dungeon/ --tui                        # styled boxes
//	go run ./examples/dungeon/ --record /tmp/dungeon.json   # save a trace
//	go run ./examples/dungeon/ --doc json                   # for embed hosts
//
// The graph deliberately includes:
//   - goblin(fight) → cave-fork(right) → goblin (cycle, MaxVisits-bounded)
//   - dark-passage(back) → cave-fork(left) → dark-passage (cycle)
//   - ring-cave → choose-dir → meadow → ring-cave (longer wandering cycle)
//   - death → entrance → ... → death (full reset cycle)
//   - convergence: both paths through the dungeon eventually reach `dragon`
//
// The ring state is Go-only: declared as a closure variable, set by
// `ring-cave`'s Run, read by `dragon`'s Run. demonstrates the canonical
// "content in md, state in Go" split.
package main

import (
	_ "embed"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/tui"
)

// Random honorifics for the cave's introductory greeting. Picked once
// at the first visit to "name"; reused on subsequent reincarnations
// because the cave has memory, in its way.
var honorifics = []string{
	"Brave",
	"Humorous",
	"Stoic",
	"Reluctant",
	"Dapper",
	"Foolhardy",
	"Inconvenient",
	"Mildly Curious",
	"Perpetually Lost",
	"Surprisingly Tall",
}

// demoMarkdown is the sidecar content. Using go:embed makes the binary
// self-contained — it works regardless of the invoker's cwd, and
// `go install` produces a single-file binary that ships its own demo.
//
//go:embed demo.md
var demoMarkdown []byte

func main() {
	demo := demokit.New("placeholder").
		FromMarkdownBytes(demoMarkdown).
		Dir("dungeon").
		MaxSteps(50).
		MaxVisits(3). // catches infinite goblin/passage loops
		AutoAcceptAfter(8 * time.Second).
		ShowCountdown(true)

	// State held in Go closures, never in the markdown:
	//   hasRing — set by ring-cave, read by dragon to decide outcome
	//   epithet — picked once at the name step, reused on revisits
	//             so the cave remembers you across deaths
	hasRing := false
	epithet := ""

	// --- navigation hub ---

	demo.Bind("entrance").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		// No-op step — falls through to the name step. Re-entered
		// from the death loop and the "try again" branch.
		return nil
	})

	demo.Bind("name").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		name := ctx.Inputs["name"].(string)
		if epithet == "" {
			epithet = honorifics[rand.IntN(len(honorifics))]
		}
		if ctx.Visits == 1 {
			fmt.Printf("(The cave murmurs: \"Welcome, %s the %s.\")\n", name, epithet)
		} else {
			fmt.Printf("(The cave murmurs: \"Back so soon, %s the %s?\")\n", name, epithet)
		}
		return nil
	})

	demo.Bind("choose-dir").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		switch ctx.Inputs["direction"] {
		case "north":
			return &demokit.StepResult{Next: "cave-fork"}
		case "south":
			return &demokit.StepResult{Next: "meadow"}
		case "east":
			return &demokit.StepResult{Next: "goblin"}
		}
		return demokit.Errf("unrecognized direction %v", ctx.Inputs["direction"])
	})

	// --- south branch (the only path to the ring) ---

	demo.Bind("meadow").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		// JSON traces widen ints to float64; accept either form.
		var coins int
		switch v := ctx.Inputs["tribute"].(type) {
		case int:
			coins = v
		case float64:
			coins = int(v)
		default:
			return demokit.Errf("unexpected tribute type %T", v)
		}
		if coins <= 0 {
			fmt.Println("(You walk away. The fountain doesn't seem to mind.)")
			return &demokit.StepResult{Next: "entrance"}
		}
		fmt.Printf("(You toss %d coin(s) in. The water glows.)\n", coins)
		return &demokit.StepResult{Next: "ring-cave"}
	})

	demo.Bind("ring-cave").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		if !hasRing {
			hasRing = true
			fmt.Println("(You feel a small weight settle in your pocket.)")
		} else {
			fmt.Println("(The pool is empty now.)")
		}
		return &demokit.StepResult{Next: "choose-dir"}
	})

	// --- north branch (the path to the dragon) ---

	demo.Bind("cave-fork").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		if ctx.Inputs["fork"] == "right" {
			return &demokit.StepResult{Next: "goblin"}
		}
		return &demokit.StepResult{Next: "dark-passage"}
	})

	demo.Bind("dark-passage").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		if ctx.Inputs["nerve"] == "forward" {
			return &demokit.StepResult{Next: "dragon"}
		}
		return &demokit.StepResult{Next: "cave-fork"}
	})

	// --- east branch (looping goblin) ---

	demo.Bind("goblin").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		if ctx.Inputs["response"] == "fight" {
			fmt.Println("(You shout something brave. The goblin sighs and wanders off.)")
			return &demokit.StepResult{Next: "cave-fork"}
		}
		fmt.Println("(You back away slowly. The goblin returns to polishing.)")
		return &demokit.StepResult{Next: "choose-dir"}
	})

	// --- the dragon (state-driven outcome) ---

	demo.Bind("dragon").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		if hasRing {
			return &demokit.StepResult{Next: "victory"}
		}
		return &demokit.StepResult{Next: "death"}
	})

	// --- terminal nodes ---

	demo.Bind("victory").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		// Skip past `death` (which is the next item in md order) to the
		// "try again?" prompt. Don't rely on fall-through here — md
		// order is content-driven, not control-flow-driven.
		return &demokit.StepResult{Next: "again"}
	})

	demo.Bind("death").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		// Reset state on the way back so the second run is fair.
		hasRing = false
		return &demokit.StepResult{Next: "entrance"}
	})

	demo.Bind("again").Run(func(ctx demokit.StepContext) *demokit.StepResult {
		if ctx.Inputs["again"] == "yes" {
			hasRing = false
			return &demokit.StepResult{Next: "entrance"}
		}
		// "no" → fall through to the `end` section.
		return nil
	})

	// `end` is a prose-only section — no Run needed.

	// --- renderer flag ---

	for _, arg := range os.Args[1:] {
		if strings.TrimSpace(arg) == "--tui" {
			demo.WithRenderer(tui.New())
		}
	}

	demo.Execute()
}
