// Command agentctl is the command-line client for operating Cursus.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

const usage = "usage: agentctl <command>\n\ncommands:\n  version   print the build version\n"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, version())
		return 0
	default:
		fmt.Fprintf(stderr, "agentctl: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

// version reports what the Go toolchain stamped into the binary, so no
// version string is maintained by hand. A binary built outside a module
// checkout, or with `go run`, reports "(devel)".
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "agentctl (unknown)"
	}
	return fmt.Sprintf("agentctl %s %s", info.Main.Version, info.GoVersion)
}
