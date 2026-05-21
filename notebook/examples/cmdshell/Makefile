.PHONY: run build test race tidy

# Launch the shell in a real terminal.
run:
	go run .

build:
	go build -o cmdshell .

test:
	go test ./...

race:
	go test -race ./...

tidy:
	go mod tidy
