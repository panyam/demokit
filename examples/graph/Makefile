.PHONY: run demo runquiet record replay gen-readme doc-md doc-html doc-json

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

# Regenerate this example's README.md from a recorded trace.
# The default-input run captures the "expired token → refresh" branch.
gen-readme: record
	go run . --doc md --from $(TRACE) > README.md
	@echo "Wrote README.md"

# Trace-driven documentation outputs.
doc-md: record
	go run . --doc md --from $(TRACE)

doc-html: record
	go run . --doc html --from $(TRACE)

# JSON for embed hosts. Includes both the static demo definition and
# the recorded trace.
doc-json: record
	go run . --doc json --from $(TRACE)
