// cmdshell is a tiny shell wrapper built on the notebook
// component. Each command you type runs through "sh -c <cmd>" and
// its combined stdout/stderr streams into a new OutputCell in
// real time.
//
// It exists to exercise the notebook against arbitrary output
// shapes — short lines, long lines, mega-line dumps (`find /`,
// `cat /etc/...`), interactive-ish streams (`ping`, `tail -f`).
// Useful for stress-testing scrolling, mouse-wheel + drag-select
// copy, viewport resize, etc.
//
// Built-in non-shell commands:
//
//	q | quit       — exit
//	clear          — remove every cell (start fresh)
//
// Everything else runs as a shell command.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/panyam/demokit/notebook"
	"github.com/panyam/demokit/notebook/cells"
)

func main() {
	nb := notebook.New(
		notebook.WithPromptFactory(cells.PromptFactory()),
		notebook.WithClipboard(notebook.OSC52Clipboard()),
	)

	go runREPL(nb)
	if err := nb.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runREPL(nb *notebook.Notebook) {
	nb.SetHeader("cmdshell", "type a shell command · q quits · clear wipes history")
	nb.Append(cells.NewNote("intro", "Try", introBody()))

	cmdIDs := []notebook.CellID{}
	n := 0
	for {
		n++
		resp := nb.AwaitInput([]notebook.Input{
			notebook.NewStringInput("cmd", "$", nil),
		})
		if resp.Source == "cancelled" {
			return
		}
		src, _ := resp.Answers["cmd"].(string)
		src = strings.TrimSpace(src)
		switch {
		case src == "":
			// ignore — loop prompts again
		case src == "q" || src == "quit":
			nb.SetDone()
			nb.Stop()
			return
		case src == "clear":
			for _, id := range cmdIDs {
				nb.Remove(id)
			}
			cmdIDs = cmdIDs[:0]
		default:
			id := runCommand(nb, n, src)
			cmdIDs = append(cmdIDs, id)
		}
	}
}

// runCommand appends an OutputCell, spawns `sh -c <src>` with
// stdout+stderr piped into the cell's Stream, waits for it to
// finish, marks the cell done. Blocks the REPL until the command
// returns — same shape as a normal shell.
func runCommand(nb *notebook.Notebook, n int, src string) notebook.CellID {
	id := notebook.CellID(fmt.Sprintf("cmd-%d", n))
	oc := cells.NewOutput(string(id), 12)
	// Manual fallback path for the iTerm "Applications may access
	// clipboard" case: OSC52 reports success but is suppressed.
	// 't' after 'c' writes the buffer to /tmp.
	oc.SetFallbackClipboard(notebook.FileClipboard(""))
	if _, err := nb.Append(oc); err != nil {
		return id
	}
	w := nb.Stream(id)
	fmt.Fprintf(w, "$ %s\n", src)

	cmd := exec.Command("sh", "-c", src)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(w, "[error] %v\n", err)
		markDone(nb, id)
		return id
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(w, stdout) }()
	go func() { defer wg.Done(); _, _ = io.Copy(w, stderr) }()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(w, "[exit] %v\n", err)
	}
	markDone(nb, id)
	return id
}

func markDone(nb *notebook.Notebook, id notebook.CellID) {
	nb.Update(id, func(c notebook.Cell) notebook.Cell {
		if oc, ok := c.(*cells.OutputCell); ok {
			oc.MarkDone()
		}
		return c
	})
}

func introBody() string {
	return strings.Join([]string{
		"  ls -la",
		"  ps aux | head -30",
		"  find . -name '*.go' -not -path '*/node_modules/*'",
		"  cat /etc/hosts",
		"  seq 1 100",
		"",
		"OutputCells default to top/bottom borders only — drag-select",
		"the body and your terminal copies just the content. Wheel-scroll",
		"within a cell when the buffer overflows; q to quit.",
	}, "\n")
}
