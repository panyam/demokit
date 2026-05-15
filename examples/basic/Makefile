.PHONY: run demo note runsmooth runquiet demo-quiet record replay gen-readme doc-md doc-html doc-json

TRACE := /tmp/demokit-basic-trace.json

# Interactive, TUI styled boxes
demo:
	go run . --tui

# Interactive, Bubble Tea notebook UI (cells + vim-modal navigation)
note:
	go run . --note

# Interactive, plain text
run:
	go run .

# Interactive, plain text with smooth scroll
runsmooth:
	go run . --smooth

# Non-interactive, plain text
runquiet:
	go run . --non-interactive

# Non-interactive, TUI styled boxes
demo-quiet:
	go run . --tui --non-interactive

# Record a non-interactive run to $(TRACE)
record:
	go run . --non-interactive --record $(TRACE)
	@echo "Trace written to $(TRACE)"

# Replay the recorded trace deterministically
replay: record
	go run . --replay $(TRACE)

# Regenerate this example's README.md from the static demo definition.
gen-readme:
	go run . --doc md > README.md
	@echo "Wrote README.md"

# Documentation outputs.
doc-md:
	go run . --doc md

doc-html:
	go run . --doc html

# JSON for embed hosts. Without --from, emits the static demo definition
# only (no trace).
doc-json:
	go run . --doc json

# Trace-driven documentation outputs.
doc-md-from: record
	go run . --doc md --from $(TRACE)

doc-html-from: record
	go run . --doc html --from $(TRACE)

doc-json-from: record
	go run . --doc json --from $(TRACE)
