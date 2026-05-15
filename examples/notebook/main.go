// Command notebook-demo exercises the Phase A.1 notebook UI in
// isolation — no demokit.Execute integration yet. It constructs a
// hand-rolled cell list (Meta + Section + multi-variant Verbatim +
// streaming Output) and runs the Bubble Tea program directly so the
// renderer bridge (PR2) can be reviewed against a known-good UI.
//
// Run with:
//
//	go run ./examples/notebook/
//
// Quit with q or Ctrl+C.
package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/tui/notebook"
)

func main() {
	cells, outputBuf, outputCell := buildCells()
	model := notebook.New(cells).
		WithQuitOnAdvance().
		WithOutputSubscription(outputBuf, outputCell.ID())

	prog := tea.NewProgram(model, tea.WithAltScreen())
	// Spawn a tiny streamer that appends lines to the OutputCell's
	// buffer once every ~400ms so the user can see live-stream
	// rendering during navigation. MarkDone runs when the stream
	// completes so the cell's "live" status flips to "end".
	go streamFakeOutput(outputBuf, outputCell)

	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "notebook demo error: %v\n", err)
		os.Exit(1)
	}
}

func buildCells() ([]notebook.Cell, *notebook.OutputBuffer, *notebook.OutputCell) {
	verb := []demokit.Variant{
		{Label: "curl", Lang: "bash", Content: `curl -s -X POST https://auth.example/oauth2/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=refresh_token&refresh_token=eyJhbGci...'`, IsDefault: true},
		{Label: "python", Lang: "python", Content: `import requests
r = requests.post("https://auth.example/oauth2/token",
  data={"grant_type": "refresh_token", "refresh_token": "eyJhbGci..."})
print(r.json())`},
		{Label: "go", Lang: "go", Content: `resp, _ := http.PostForm("https://auth.example/oauth2/token",
  url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"eyJhbGci..."}})
defer resp.Body.Close()`},
	}

	buf := notebook.NewOutputBuffer()
	outputCell := notebook.NewOutputCell("step.refresh#0.output", buf, 10)

	cells := []notebook.Cell{
		notebook.NewMetaCell(
			"step.refresh#0.meta",
			"Refresh the token",
			"The access_token expires after 3600 seconds. Use the refresh_token to mint a fresh one without re-prompting the user.\n\nThe IdP returns a new access_token plus a (possibly rotated) refresh_token.",
		),
		notebook.NewSectionCell(
			"step.refresh#0.section0",
			"Heads up",
			"Refresh tokens are long-lived but revocable. Treat them like passwords — never log them, and rotate on every refresh if the IdP supports it.",
		),
		notebook.NewVerbatimCell(
			"step.refresh#0.verbatim0",
			"Refresh token request",
			verb,
		),
		outputCell,
	}
	return cells, buf, outputCell
}

func streamFakeOutput(buf *notebook.OutputBuffer, c *notebook.OutputCell) {
	lines := []string{
		`HTTP/1.1 200 OK`,
		`content-type: application/json`,
		``,
		`{`,
		`  "access_token": "eyJraWQ...",`,
		`  "token_type": "Bearer",`,
		`  "expires_in": 3600,`,
		`  "refresh_token": "eyJhbGci..."`,
		`}`,
	}
	for _, line := range lines {
		time.Sleep(400 * time.Millisecond)
		buf.Append([]byte(line + "\n"))
	}
	c.MarkDone()
}
