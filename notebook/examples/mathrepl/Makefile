.PHONY: run build test race tidy

# Launch the REPL in a real terminal.
run:
	go run .

# Build the binary into ./mathrepl (gitignored).
build:
	go build -o mathrepl .

# Run unit tests for this module.
test:
	go test ./...

# Run the unit tests with the race detector.
race:
	go test -race ./...

# Tidy this module's go.mod.
tidy:
	go mod tidy
