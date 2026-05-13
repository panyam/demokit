.PHONY: run demo runquiet record replay gen-readme doc-md doc-md-default doc-md-python doc-html doc-json doc-bundle

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
# --variant=all so multi-variant verbatim blocks show every form in
# the generated doc (e.g. curl/python/go for the refresh step); the
# Go default marker on the curl variant still drives TUI keyboard copy.
gen-readme: record
	go run . --doc md --from $(TRACE) --variant=all > README.md
	@echo "Wrote README.md"

# Trace-driven documentation outputs.
# --variant flag demos:
#   doc-md          renders every variant of every multi-variant block.
#   doc-md-default  honors the Go-side Default() marker (curl only on
#                   the refresh step) — matches what an unset --variant
#                   produces when a Default is declared.
#   doc-md-python   filters to just the variant labeled python; useful
#                   for producing language-specific cheat-sheets from a
#                   single demo definition.
doc-md: record
	go run . --doc md --from $(TRACE) --variant=all

doc-md-default: record
	go run . --doc md --from $(TRACE) --variant=default

doc-md-python: record
	go run . --doc md --from $(TRACE) --variant=python

doc-html: record
	go run . --doc html --from $(TRACE) --variant=all

# JSON for embed hosts. Includes both the static demo definition and
# the recorded trace.
doc-json: record
	go run . --doc json --from $(TRACE)

# Self-contained HTML bundle (player + CSS + trace, all inlined).
# Open the resulting file in any browser; works from file://.
doc-bundle: record
	go run . --doc bundle --from $(TRACE) --out /tmp/graph.html
	@echo "Bundle: /tmp/graph.html — open in browser"
