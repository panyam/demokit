.PHONY: run demo note serve runquiet record replay doc-md doc-html doc-json doc-bundle gen-readme

TRACE := /tmp/demokit-dungeon-trace.json
SERVE_ADDR := :8765

# Interactive, plain text — the canonical "play through the cave" mode.
run:
	go run .

# Interactive, TUI styled boxes.
demo:
	go run . --tui

# Interactive, Bubble Tea notebook UI (cells + vim-modal navigation).
note:
	go run . --note

# Live web mode — serves the demo at http://localhost$(SERVE_ADDR)/.
# Browser drives inputs via WebSocket; server terminal mirrors the
# step output. Override the address with `make serve SERVE_ADDR=:9000`.
serve:
	go run . --serve $(SERVE_ADDR)

servetui:
	go run . --tui --serve $(SERVE_ADDR)

# Non-interactive run using each input's WithDefault. Default path:
#   north → left → forward → dragon → death (no ring)
#   → entrance → choose-dir → ... → death again until MaxSteps trips
# This deliberately demonstrates the "no ring → dragon eats you" branch.
runquiet:
	go run . --non-interactive

# Record a non-interactive run to $(TRACE). The default-input path
# loops on the no-ring death until MaxSteps fires — produces a trace
# that exercises the cycle and the safety guard.
record:
	go run . --non-interactive --record $(TRACE)
	@echo "Trace written to $(TRACE)"

# Replay the recorded trace deterministically.
replay: record
	go run . --replay $(TRACE)

# Documentation outputs.
doc-md: record
	go run . --doc md --from $(TRACE)

doc-html: record
	go run . --doc html --from $(TRACE)

# JSON for embed hosts. Includes both the static demo definition and
# the recorded trace.
doc-json: record
	go run . --doc json --from $(TRACE)

# Self-contained HTML bundle (player + CSS + trace, all inlined).
# Open the resulting file in any browser; works from file://.
doc-bundle: record
	go run . --doc bundle --from $(TRACE) --out /tmp/dungeon.html
	@echo "Bundle: /tmp/dungeon.html — open in browser"

# Regenerate this example's README.md from a recorded trace. The
# default-input run captures the no-ring → death loop, which makes
# for a nice "what happens when you die" walkthrough.
gen-readme: record
	go run . --doc md --from $(TRACE) > README.md
	@echo "Wrote README.md"
