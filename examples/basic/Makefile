.PHONY: run runtui runsmooth runquiet runtui-quiet readme

# Interactive, plain text
run:
	go run .

# Interactive, TUI styled boxes
runtui:
	go run . --tui

# Interactive, plain text with smooth scroll
runsmooth:
	go run . --smooth

# Non-interactive, plain text
runquiet:
	go run . --non-interactive

# Non-interactive, TUI styled boxes
runtui-quiet:
	go run . --tui --non-interactive

# Generate README from demo definitions
readme:
	go run . --readme
