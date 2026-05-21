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
// Two ways to drive the shell:
//
//  1. PromptCell at the bottom of the main list (the default REPL
//     loop) — types a command and runs it.
//  2. Vim-style ":" — press ":" in navigation mode to open a
//     CommandCell docked at the viewport bottom; Enter runs the
//     command, Esc cancels. Demonstrates the DockedCell API. The
//     command runs synchronously, so during long jobs the ":"
//     prompt won't accept new input — switch back to the main
//     PromptCell for parallel work.
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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
	"github.com/panyam/demokit/notebook/cells"
)

func main() {
	repl := &shellState{}
	km := notebook.DefaultKeyMap()
	// ":" in navigation mode opens a vim-style command bar at the
	// Bottom dock. Apps choose the trigger key — the framework
	// only ships the OpenCommandBar convenience.
	km.Modes[notebook.NavigationMode][":"] = func(nb *notebook.Notebook) tea.Cmd {
		return cells.OpenCommandBar(nb, ":", func(src string) {
			// onSubmit runs in the deferred tea.Cmd goroutine, so
			// the BT loop is already free — but we still don't
			// want to block the cmd goroutine on a long-running
			// shell command, so kick off the work in its own
			// goroutine and return.
			go repl.runFromCommandBar(nb, src)
		})
	}

	nb := notebook.New(
		notebook.WithPromptFactory(cells.PromptFactory()),
		notebook.WithClipboard(notebook.OSC52Clipboard()),
		notebook.WithKeyMap(km),
	)
	repl.nb = nb

	go runREPL(nb, repl)
	if err := nb.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// shellState holds the bits the ":" command-bar handler needs to
// share with the prompt-driven REPL loop. The REPL goroutine
// owns the command counter; the command-bar handler bumps it
// under a mutex.
type shellState struct {
	nb     *notebook.Notebook
	mu     sync.Mutex
	n      int
	cmdIDs []notebook.CellID
}

// runFromCommandBar handles a single ":<cmd>" submission from the
// docked CommandCell. Same lifecycle as the prompt path but
// triggered out-of-band so the REPL doesn't have to know about
// the dock.
func (s *shellState) runFromCommandBar(nb *notebook.Notebook, src string) {
	src = strings.TrimSpace(src)
	if src == "" {
		return
	}
	s.mu.Lock()
	s.n++
	n := s.n
	s.mu.Unlock()
	id := runCommand(nb, n, src)
	s.mu.Lock()
	s.cmdIDs = append(s.cmdIDs, id)
	s.mu.Unlock()
}

func runREPL(nb *notebook.Notebook, repl *shellState) {
	nb.SetHeader("cmdshell", "type a shell command · : opens command bar · q quits · clear wipes history")
	nb.Append(cells.NewNote("intro", "Try", introBody()))

	for {
		repl.mu.Lock()
		repl.n++
		n := repl.n
		repl.mu.Unlock()

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
			repl.mu.Lock()
			ids := repl.cmdIDs
			repl.cmdIDs = repl.cmdIDs[:0]
			repl.mu.Unlock()
			for _, id := range ids {
				nb.Remove(id)
			}
		default:
			id := runCommand(nb, n, src)
			repl.mu.Lock()
			repl.cmdIDs = append(repl.cmdIDs, id)
			repl.mu.Unlock()
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
		"",
		"Press ':' (in navigation mode — press Esc to leave the prompt",
		"first) to open the vim-style command bar at the bottom. The",
		"command bar grows as you type. Enter runs, Esc cancels.",
	}, "\n")
}
