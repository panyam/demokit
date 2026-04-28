.PHONY: run run-tui run-quiet run-tui-quiet readme

# Interactive, plain text
run:
	go run .

# Interactive, TUI styled boxes
runtui:
	go run . --tui

# Non-interactive, plain text
runquiet:
	go run . --non-interactive

# Non-interactive, TUI styled boxes
runtui-quiet:
	go run . --tui --non-interactive

# Generate README from demo definitions
readme:
	go run . --readme
