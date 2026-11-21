// Command agentctl is the command-line client for operating Cursus.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/AndersonMonteiro/cursus/internal/auth"
	"github.com/AndersonMonteiro/cursus/internal/config"
)

const usage = "usage: agentctl <command> [flags]\n\ncommands:\n" +
	"  version   print the build version\n" +
	"  token     mint a development token; see agentctl token -h\n"

const defaultTokenTTL = time.Hour

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage) //nolint:errcheck // terminal write
		return 2
	}
	switch args[0] {
	case "version":
		if len(args) != 1 {
			fmt.Fprint(stderr, usage) //nolint:errcheck // terminal write
			return 2
		}
		fmt.Fprintln(stdout, version()) //nolint:errcheck // terminal write
		return 0
	case "token":
		return token(args[1:], getenv, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "agentctl: unknown command %q\n\n%s", args[0], usage) //nolint:errcheck // terminal write
		return 2
	}
}

// roleFlags collects a repeatable --role flag.
type roleFlags []auth.Role

func (r *roleFlags) String() string { return fmt.Sprint(*r) }

func (r *roleFlags) Set(value string) error {
	role, err := auth.ParseRole(value)
	if err != nil {
		return err
	}
	*r = append(*r, role)
	return nil
}

// token mints a token with the key the API verifies with. The key is read
// from the environment and never from a flag, which would leave it in the
// shell history and the process list.
func token(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("agentctl token", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var roles roleFlags
	subject := flags.String("subject", "", "subject the token is issued to (required)")
	tenant := flags.String("tenant", "", "tenant the subject acts for (required)")
	flags.Var(&roles, "role", "role to grant: spec_author, owner, approver, or client_service (required, repeatable)")
	ttl := flags.Duration("ttl", defaultTokenTTL, "lifetime of the token")
	if err := flags.Parse(args); err != nil {
		// Parse has already printed the usage or the error.
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	fail := func(message string) int {
		fmt.Fprintf(stderr, "agentctl token: %s\n", message) //nolint:errcheck // terminal write
		return 2
	}
	switch {
	case flags.NArg() > 0:
		return fail(fmt.Sprintf("unexpected argument %q", flags.Arg(0)))
	case *subject == "":
		return fail("--subject is required")
	case *tenant == "":
		return fail("--tenant is required")
	case len(roles) == 0:
		return fail("--role is required")
	}

	key := getenv(config.EnvTokenSigningKey)
	if key == "" {
		return fail(config.EnvTokenSigningKey + " is not set")
	}
	issuer, err := auth.NewIssuer([]byte(key), time.Now)
	if err != nil {
		return fail(config.EnvTokenSigningKey + ": " + err.Error())
	}
	signed, err := issuer.Issue(*subject, *tenant, roles, *ttl)
	if err != nil {
		return fail(err.Error())
	}
	fmt.Fprintln(stdout, signed) //nolint:errcheck // terminal write
	return 0
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
