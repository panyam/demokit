.PHONY: run demo runsmooth runquiet demo-quiet readme record replay readme-from readme-html-from gen-readme

TRACE := /tmp/demokit-basic-trace.json

# Interactive, TUI styled boxes
demo:
	go run . --tui

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

# Generate README from demo definitions (linear, mermaid)
readme:
	go run . --readme

# Record a non-interactive run to $(TRACE)
record:
	go run . --non-interactive --record $(TRACE)
	@echo "Trace written to $(TRACE)"

# Replay the recorded trace deterministically
replay: record
	go run . --replay $(TRACE)

# Render markdown of the recorded path (captures actual visited steps,
# inputs, and outputs — different from --readme which is static)
readme-from: record
	go run . --readme-from $(TRACE)

# Same, but as standalone HTML
readme-html-from: record
	go run . --readme-html-from $(TRACE)

# Regenerate this example's README.md from the demo definition
gen-readme:
	go run . --readme > README.md
	@echo "Wrote README.md"
