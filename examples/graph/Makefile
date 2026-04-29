.PHONY: run demo runquiet record replay readme-from readme-html-from gen-readme

TRACE := /tmp/demokit-graph-trace.json

# Interactive, TUI styled boxes
demo:
	go run . --tui

# Interactive, plain text — pick a symptom and walk the recovery path
run:
	go run .

# Non-interactive — uses each input's WithDefault
runquiet:
	go run . --non-interactive

# Record a non-interactive run to $(TRACE).
# Inputs default to "expired" symptom and "yes" to refresh, so the
# recorded path covers the expired-token recovery branch.
record:
	go run . --non-interactive --record $(TRACE)
	@echo "Trace written to $(TRACE)"

# Replay the recorded trace deterministically (same path, same inputs).
replay: record
	go run . --replay $(TRACE)

# Render markdown of the recorded path. This is the canonical way to
# document a branching demo — one trace = one story.
readme-from: record
	go run . --readme-from $(TRACE)

# Same, but as standalone HTML.
readme-html-from: record
	go run . --readme-html-from $(TRACE)

# Regenerate this example's README.md from a recorded trace.
# The default-input run captures the "expired token → refresh" branch.
gen-readme: record
	go run . --readme-from $(TRACE) > README.md
	@echo "Wrote README.md"
