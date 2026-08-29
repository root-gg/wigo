package main

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/root-gg/wigo/src/wigo"
)

// The way out of the circle.
//
// Every other way of minting a token goes through the api, which needs a
// credential, which is what a first token is for. Run here, on the machine, it
// owes nobody an authentication : whoever can read wigo's database can already
// read every secret it holds.

const tokenUsage = `wigo token

Usage:
	wigo token create <name> [--role=ROLE] [--for=DURATION] [--config=CONFIG]
	wigo token list [--config=CONFIG]
	wigo token revoke <id> [--config=CONFIG]

Options:
	--role=ROLE         readonly or operator [default: operator]
	--for=DURATION      How long it is good for, like 24h or 720h. Empty never
	                    expires, which is right for a scraper and wrong for
	                    somebody you are lending it to for an afternoon
	--config=CONFIG     Configuration file [default: /etc/wigo/wigo.conf]

Reads and writes the database directly, so it works when the api cannot be
reached or cannot be authenticated against -- which is the situation a first
token exists to get out of. Run it on the machine, as whoever owns the
database.
`

// runTokenCommand returns what the process should exit with.
func runTokenCommand(args []string) int {
	opts, err := docopt.ParseArgs(tokenUsage, args, wigo.Version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	configFile, _ := opts.String("--config")
	if err := wigo.OpenForTokens(configFile); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	if create, _ := opts.Bool("create"); create {
		return createToken(opts)
	}
	if list, _ := opts.Bool("list"); list {
		return listTokens()
	}

	return revokeToken(opts)
}

func createToken(opts docopt.Opts) int {
	name, _ := opts.String("<name>")
	role, _ := opts.String("--role")
	forDuration, _ := opts.String("--for")

	expiresAt, err := wigo.ParseTokenExpiry(forDuration)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	token, secret, err := wigo.CreateApiToken(name, role, expiresAt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	// Only its sha256 is kept, so this is the one and only time it can be
	// printed. Saying so is the difference between somebody copying it now and
	// somebody minting a second one in five minutes.
	fmt.Printf("%s\n\n", secret)
	fmt.Printf("Token %d, named %q, %s.\n", token.Id, token.Name, describeExpiry(token))
	fmt.Printf("Only its sha256 is stored, so this is the only time it is shown.\n")
	fmt.Printf("Present it as:  Authorization: Bearer %s\n", secret)

	return 0
}

func listTokens() int {
	tokens, err := wigo.ApiTokens()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	if len(tokens) == 0 {
		fmt.Println("No tokens.")
		return 0
	}

	// Revoked ones are listed too : what a token was called and when it was
	// turned off is a question somebody asks later.
	out := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(out, "ID\tNAME\tROLE\tSTATE\tLAST USED")

	now := time.Now().Unix()
	for _, token := range tokens {
		fmt.Fprintf(out, "%d\t%s\t%s\t%s\t%s\n",
			token.Id, token.Name, token.Role, describeState(token, now), describeLastUsed(token))
	}

	if err := out.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	return 0
}

func revokeToken(opts docopt.Opts) int {
	raw, _ := opts.String("<id>")

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid token id %q\n", raw)
		return 1
	}

	if err := wigo.RevokeApiToken(id); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	fmt.Printf("Token %d revoked.\n", id)

	return 0
}

func describeExpiry(token wigo.ApiToken) string {
	if token.ExpiresAt == 0 {
		return "no expiry"
	}

	return fmt.Sprintf("expires %s", time.Unix(token.ExpiresAt, 0).Format(time.RFC1123))
}

func describeState(token wigo.ApiToken, now int64) string {
	switch {
	case token.RevokedAt > 0:
		return fmt.Sprintf("revoked %s", time.Unix(token.RevokedAt, 0).Format("2006-01-02"))
	case token.ExpiresAt > 0 && token.ExpiresAt <= now:
		return fmt.Sprintf("expired %s", time.Unix(token.ExpiresAt, 0).Format("2006-01-02"))
	case token.ExpiresAt > 0:
		return fmt.Sprintf("until %s", time.Unix(token.ExpiresAt, 0).Format("2006-01-02"))
	default:
		return "no expiry"
	}
}

func describeLastUsed(token wigo.ApiToken) string {
	if token.LastUsed == 0 {
		return "never"
	}

	return time.Unix(token.LastUsed, 0).Format("2006-01-02 15:04")
}
