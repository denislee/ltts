package main

import (
	"fmt"
	"os"
)

func main() {
	if err := runTUI(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
