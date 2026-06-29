// Command demokit scaffolds and migrates demokit walkthroughs.
//
//	demokit init [dir]                       scaffold a project (Makefile + sample)
//	demokit new <name> [--kind=KIND] [--dir] add one example from a starter
//	demokit extract <file.go>                Go walkthrough -> sidecar markdown
//
// KIND is narrated | live | branching (default live). Install with:
//
//	go install github.com/panyam/demokit/cmd/demokit@latest
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "new":
		err = runNew(args)
	case "extract":
		err = runExtract(args)
	case "help", "-h", "--help":
		usage(os.Stdout)
		return
	default:
		fmt.Fprintf(os.Stderr, "demokit: unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "demokit %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `demokit — scaffold and migrate walkthroughs

Usage:
  demokit init [dir]                          scaffold a project (Makefile + sample)
  demokit new <name> [--kind=KIND] [--dir D]  add one example from a starter
  demokit extract <file.go>                   Go walkthrough -> sidecar markdown + Bind skeleton

KIND is one of: narrated | live | branching   (default: live)
  narrated   sidecar markdown only, no Go behavior
  live       markdown content + Go behavior bound by step id
  branching  Go-driven routing and state
`)
}
