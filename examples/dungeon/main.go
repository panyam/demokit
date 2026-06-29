// Cave of Cards — a sidecar-markdown adventure that stresses the
// directed-graph machinery: multiple cycles, a converging join, and
// one piece of Go-side state (a magic ring) that decides the outcome
// at the dragon. All prose lives in demo.md; this file wires Run
// closures via Bind, threads the ring state, and starts the demo.
//
// Try:
//
//	go run ./examples/dungeon/                              # interactive
//	go run ./examples/dungeon/ --mode=tui                   # styled boxes (also --tui)
//	go run ./examples/dungeon/ --mode=notebook              # Bubble Tea notebook UI
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
	"time"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/harness"
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

// ANSI escape codes for the dragon scene. Plain text terminals
// render these as literal characters; modern TTYs interpret them.
// The streaming demo prints them as-is — TTY-stripping for piped
// output is a future enhancement (would key off term.IsTerminal).
const (
	ansiReset      = "\033[0m"
	ansiDim        = "\033[2m"
	ansiBold       = "\033[1m"
	ansiCyan       = "\033[36m"
	ansiBoldYellow = "\033[1;33m"
	ansiRed        = "\033[31m"
	ansiBoldRed    = "\033[1;31m"
	ansiGold       = "\033[38;5;220m" // 256-color: warm gold
	ansiSmoke      = "\033[38;5;245m" // 256-color: medium gray
	ansiScale      = "\033[38;5;88m"  // 256-color: deep crimson
)

// dragonVerdict is the dragon step's branching tail — same
// outcome whether the user watched the full reveal or skipped
// past it with Enter. Lifted out so both code paths share the
// decision; ring presence stays the only state input.
func dragonVerdict(hasRing bool) *demokit.StepResult {
	if hasRing {
		return &demokit.StepResult{Next: "victory"}
	}
	return &demokit.StepResult{Next: "death"}
}

// dragonScene is revealed line-by-line in the dragon step, exercising
// streaming output: each line prints with a brief sleep so the
// silhouette assembles in front of the user instead of dumping all
// at once when Run returns. Three movements: approach (stalactites),
// reveal (the dragon itself), aftermath (treasure, magazine, "lunch").
var dragonScene = []string{
	ansiSmoke + "      The cave widens. Your torch flickers and gives up entirely." + ansiReset,
	ansiSmoke + "      Something else takes over the lighting." + ansiReset,
	"",
	ansiCyan + "                    /\\        /\\        /\\        /\\" + ansiReset,
	ansiCyan + "                   /  \\      /  \\      /  \\      /  \\" + ansiReset,
	ansiCyan + "                  /    \\    /    \\    /    \\    /    \\" + ansiReset,
	ansiCyan + "                 /______\\  /______\\  /______\\  /______\\" + ansiReset,
	"",
	ansiScale + "                              \\||/" + ansiReset,
	ansiScale + "                              |  " + ansiBoldYellow + "@" + ansiScale + "___oo" + ansiReset,
	ansiScale + "                    /\\  /\\   / (__,,,,|" + ansiReset,
	ansiScale + "                   ) /^\\) ^\\/ _)" + ansiReset,
	ansiScale + "                   )   /^\\/   _)" + ansiReset,
	ansiScale + "                   )   _ /  / _)" + ansiReset,
	ansiScale + "                  /\\  )/\\/ ||  | )_)" + ansiReset,
	ansiScale + "                 <  >      |(,,) )__)" + ansiReset,
	ansiScale + "                  ||      /    \\)___)\\" + ansiReset,
	ansiScale + "                  | \\____(      )___) )___" + ansiReset,
	ansiScale + "                   \\______(_______;;; __;;;" + ansiReset,
	"",
	ansiGold + "      Around him: rivers of gold, sparkling like dirty fountains." + ansiReset,
	ansiDim + "      A magazine titled \"" + ansiReset + ansiBold + "Hoard Quarterly" + ansiReset + ansiDim + "\" rests across one claw." + ansiReset,
	ansiDim + "      He licks a page with a forked tongue and turns it." + ansiReset,
	ansiDim + "      The article is a six-page spread on adamantine polishing." + ansiReset,
	"",
	ansiBoldYellow + "      Slowly, two enormous yellow eyes lift to meet yours." + ansiReset,
	"",
	ansiBoldRed + "      \"Oh good,\"" + ansiReset + ansiRed + " he says, conversationally. " + ansiBoldRed + "\"Lunch.\"" + ansiReset,
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

	// A cancellable streaming step: the cave whispers a few hints
	// over the course of a few seconds. Press Enter to move on
	// early; otherwise the whispers stop on their own. Timeout is a
	// safety limit (loop completes well before it fires).
	demo.Bind("listen").Timeout(8 * time.Second).Cancellable(true).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			// On revisits (death loop, "try again"), skip the whispers
			// so the demo doesn't drag every reset.
			if ctx.Visits > 1 {
				fmt.Println("(You keep walking. The cave has nothing new to say.)")
				return nil
			}
			whispers := []string{
				ansiSmoke + "    drip... drip..." + ansiReset,
				ansiSmoke + "    (faint scrabbling — claws on stone, somewhere west)" + ansiReset,
				ansiSmoke + "    (a slow leathery breath, deep in the rock)" + ansiReset,
				ansiSmoke + "    (coins shifting; the dragon is restless tonight)" + ansiReset,
				ansiSmoke + "    (a dragonfly bumps into a stalactite and apologizes)" + ansiReset,
			}
			for _, w := range whispers {
				select {
				case <-ctx.Ctx.Done():
					fmt.Println(ansiDim + "(You shake your head and step away.)" + ansiReset)
					return nil
				case <-time.After(700 * time.Millisecond):
					fmt.Println(w)
				}
			}
			fmt.Println(ansiDim + "(The cave seems to forget about you.)" + ansiReset)
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

	// Cancellable: pressing Enter during the reveal skips the
	// remaining lines and jumps straight to the verdict. The
	// shape mirrors mathrepl's `series` cancellation — both
	// loops select on a Done channel between iterations and
	// short-circuit on cancel. demokit's Cancellable wires
	// Enter → ctx.Ctx cancel; the notebookbridge's repaint tick
	// surfaces each Println within one frame.
	demo.Bind("dragon").Cancellable(true).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			for _, line := range dragonScene {
				select {
				case <-ctx.Ctx.Done():
					fmt.Println("…(you can't bear to look any longer)")
					return dragonVerdict(hasRing)
				default:
				}
				fmt.Println(line)
				select {
				case <-time.After(80 * time.Millisecond):
				case <-ctx.Ctx.Done():
					fmt.Println("…(you can't bear to look any longer)")
					return dragonVerdict(hasRing)
				}
			}
			// Beat for dramatic effect before the verdict.
			select {
			case <-time.After(700 * time.Millisecond):
			case <-ctx.Ctx.Done():
			}
			return dragonVerdict(hasRing)
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

	// --mode=plain (default) | tui | notebook (--tui / --note aliases).
	// harness.Run wires the renderer, enables --doc bundle / --serve,
	// then executes.
	harness.Run(demo)
}
