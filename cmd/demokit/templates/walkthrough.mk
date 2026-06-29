# walkthrough.mk — base targets for a demokit walkthrough.
#
# A per-example Makefile includes this fragment (adjust the path depth):
#
#   include ../walkthrough.mk
#
# Override before the include if needed:
#   DEMO_BIN      binary name for `make build`  (default: directory name)
#   RECORD_TRACE  scratch trace path            (default: /tmp/<bin>.trace.json)

DEMO_BIN     ?= $(notdir $(CURDIR))
RECORD_TRACE ?= /tmp/$(DEMO_BIN).trace.json

demo: ## Run the walkthrough (interactive, TUI)
	go run . --tui

note: ## Run in notebook mode (Bubble Tea cells)
	go run . --note

run: ## Run in plain mode
	go run .

readme: ## Regenerate WALKTHROUGH.md from the demo definition (static)
	go run . --doc md > WALKTHROUGH.md

record: ## Record a trace (interactive; press Enter to advance each step)
	go run . --tui --record $(RECORD_TRACE)
	@echo "Trace written to $(RECORD_TRACE)"

bundle: ## Build a self-contained HTML player from the recorded trace
	@mkdir -p bundle
	go run . --doc bundle --from $(RECORD_TRACE) --out bundle/index.html

build: ## Build the example binary
	go build -o $(DEMO_BIN) .

.PHONY: demo note run readme record bundle build
.DEFAULT_GOAL := demo
