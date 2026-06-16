.PHONY: demo demo-full demo-horizontal demo-none demo-quiet compare

# Default: TUI with horizontal border — the new copy-paste-friendly mode.
demo: demo-horizontal

# Side border drawn → content fit to width and soft-wrapped (the old
# behavior; mouse-select picks up the wrap, copy-paste is broken).
demo-full:
	go run . --border full

# Top/bottom rules only → content emitted raw, byte-exact for paste.
demo-horizontal:
	go run . --border horizontal

# No rules → raw content flush with scrollback.
demo-none:
	go run . --border none

# Non-interactive — skips pause prompts; same horizontal default.
demo-quiet:
	go run . --border horizontal --non-interactive

# Run both modes back-to-back so reviewers see the contrast in one
# scroll. --border full first (the wrap problem), then horizontal
# (the fix).
compare:
	@echo "=== --border full (framed, soft-wrapped) ==="
	@go run . --border full --non-interactive
	@echo
	@echo "=== --border horizontal (raw, byte-exact) ==="
	@go run . --border horizontal --non-interactive
