package main

import (
	"fmt"
	"os"
)

func main() {
	opts, cliMode, err := parseFlags(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}
	if cliMode {
		if err := runCLI(opts); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if err := runTUI(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
