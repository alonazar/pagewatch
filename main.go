// pagewatch checks web pages on a schedule and alerts you (phone
// notification, SMS or email) when words you care about appear on them.
package main

import (
	"fmt"
	"os"
)

// version is set at build time by the release workflow.
var version = "dev"

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "check":
		// Run by the scheduler at the chosen interval; no window, no questions.
		attachParentConsole()
		os.Exit(cmdCheck())
	case "", "menu":
		interactive(false)
	case "setup":
		interactive(true)
	case "version", "--version", "-v":
		attachParentConsole()
		fmt.Println("pagewatch", version)
	case "help", "--help", "-h":
		attachParentConsole()
		printHelp()
	default:
		attachParentConsole()
		fmt.Printf("Unknown command %q.\n\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Print(`pagewatch watches web pages for words and alerts you when they appear.

Usage:
  pagewatch            Open pagewatch (first time: set it up)
  pagewatch setup      Run the setup questions again
  pagewatch check      Check the pages once (this is what the schedule runs)
  pagewatch version    Show the version
`)
}
