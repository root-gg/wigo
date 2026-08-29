package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/root-gg/wigo/src/wigo"
)

var command string = ""
var probe string = ""
var wigoHost string = "localhost"
var showOnlyErrors bool = true

func main() {

	// Usage
	usage := fmt.Sprintf(`wigocli %s

Usage:
	wigocli [options]
	wigocli [options] detail
	wigocli [options] probe <probe>
	wigocli [options] remote <wigo>
	wigocli [options] remote <wigo> detail
	wigocli [options] remote <wigo> probe <probe>
	wigocli -h | --help
	wigocli --version

Commands:
	detail                  Show everything, not only what is wrong

Options:
	-h --help               Show help
	--version               Show version
	--config=CONFIG         Configuration file [default: /etc/wigo/wigo.conf]
	--json                  Print what was asked for as json, not as a summary
	--group=GROUP           Only hosts in this group
	--status=STATUS         Only what is at or above this status. A level name
	                        (ok, info, warning, critical, error) or a number
	--watch=SECONDS         Print again every SECONDS until interrupted

Exit codes are the ones a monitoring scheduler expects, taken from the worst
thing shown : 0 ok, 1 warning, 2 critical, 3 unknown. Unreachable is 3, since
not being able to ask is not the same as being told everything is fine.
`, wigo.Version)

	// Parse args
	opts, _ := docopt.ParseArgs(usage, os.Args[1:], wigo.Version)

	configFile, _ := opts.String("--config")
	showDetails, _ := opts.Bool("detail")
	showOnlyErrors = !showDetails
	probe, _ := opts.String("<probe>")
	wigoHostname, _ := opts.String("<wigo>")
	if wigoHostname != "" {
		wigoHost = wigoHostname
	}

	asJson, _ := opts.Bool("--json")
	group, _ := opts.String("--group")

	rawStatus, _ := opts.String("--status")
	minStatus, ok := wigo.ParseStatus(rawStatus)
	if !ok {
		fmt.Fprintf(os.Stderr, "Invalid --status %q : expected a level name or a number\n", rawStatus)
		os.Exit(wigo.ExitUnknown)
	}

	// Watching is the one mode with no exit code to give : it does not end.
	every, err := watchInterval(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(wigo.ExitUnknown)
	}

	apiUrl, err := apiUrlFrom(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(wigo.ExitUnknown)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	selection := wigo.Selection{Group: group, MinStatus: minStatus}

	if every > 0 {
		for {
			// Home and clear, so a screen left over from the previous pass
			// cannot be read as the current one.
			fmt.Print("\033[H\033[2J")
			if _, err := report(httpClient, apiUrl, selection, probe, asJson); err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}
			time.Sleep(every)
		}
	}

	worst, err := report(httpClient, apiUrl, selection, probe, asJson)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(wigo.ExitUnknown)
	}

	os.Exit(wigo.NagiosExitCode(worst))
}

func watchInterval(opts docopt.Opts) (time.Duration, error) {
	raw, _ := opts.String("--watch")
	if raw == "" {
		return 0, nil
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 1 {
		return 0, fmt.Errorf("invalid --watch %q : expected a number of seconds, at least 1", raw)
	}

	return time.Duration(seconds) * time.Second, nil
}

// apiUrlFrom reads where this wigo answers, from its own configuration file.
func apiUrlFrom(configFile string) (string, error) {
	config := wigo.NewConfig(configFile)
	if !config.Http.Enabled {
		return "", fmt.Errorf("the http server is not enabled in %s, so there is nothing to ask", configFile)
	}

	address := config.Http.Address
	if address == "0.0.0.0" {
		address = "127.0.0.1"
	}

	protocol := "http"
	if config.Http.SslEnabled {
		protocol = "https"
	}

	if config.Http.Login != "" && config.Http.Password != "" {
		return fmt.Sprintf("%s://%s:%s@%s:%d/api", protocol,
			config.Http.Login, config.Http.Password, address, config.Http.Port), nil
	}

	return fmt.Sprintf("%s://%s:%d/api", protocol, address, config.Http.Port), nil
}

// report fetches, narrows, prints, and says how bad what it printed was.
func report(client *http.Client, apiUrl string, selection wigo.Selection, probe string, asJson bool) (int, error) {
	resp, err := client.Get(apiUrl)
	if err != nil {
		return 0, fmt.Errorf("cannot reach wigo : %s", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("cannot read what wigo answered : %s", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("wigo answered %d : %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	wigoObj, err := wigo.NewWigoFromJson(body, 0)
	if err != nil {
		return 0, fmt.Errorf("cannot read what wigo answered : %s", err)
	}

	// One probe was asked for : nothing else is the answer, filters included.
	if probe != "" {
		return reportProbe(wigoObj, probe, asJson)
	}

	if wigoHost != "" && wigoHost != "localhost" {
		tmp, found := wigoObj.RemoteWigos.Get(wigoHost)
		if !found {
			return 0, fmt.Errorf("no remote wigo named %s", wigoHost)
		}
		wigoObj = tmp.(*wigo.Wigo)
	}

	wigoObj.Apply(selection)

	if asJson {
		return wigoObj.WorstStatus(), printJson(wigoObj)
	}

	// The summary prints the host's own global status in its header, which is
	// the whole host and not the selection. Saying what was narrowed down to
	// keeps that header from reading as a contradiction of the exit code.
	if narrowed := describeSelection(selection); narrowed != "" {
		fmt.Printf("%s\n\n", narrowed)
	}

	fmt.Print(wigoObj.GenerateSummary(showOnlyErrors))

	return wigoObj.WorstStatus(), nil
}

func reportProbe(wigoObj *wigo.Wigo, probe string, asJson bool) (int, error) {
	host := wigoObj.GetLocalHost()

	if wigoHost != "" && wigoHost != "localhost" {
		tmp, found := wigoObj.RemoteWigos.Get(wigoHost)
		if !found {
			return 0, fmt.Errorf("no remote wigo named %s", wigoHost)
		}
		host = tmp.(*wigo.Wigo).GetLocalHost()
	}

	tmp, found := host.Probes.Get(probe)
	if !found {
		return 0, fmt.Errorf("no probe named %s on %s", probe, host.Name)
	}

	result := tmp.(*wigo.ProbeResult)

	if asJson {
		return result.Status, printJson(result)
	}

	fmt.Println(result.Summary())

	return result.Status, nil
}

func describeSelection(selection wigo.Selection) string {
	parts := make([]string, 0, 2)

	if selection.Group != "" {
		parts = append(parts, fmt.Sprintf("group %s", selection.Group))
	}
	if selection.MinStatus > 0 {
		parts = append(parts, fmt.Sprintf("status %d and above", selection.MinStatus))
	}

	if len(parts) == 0 {
		return ""
	}

	return "Showing only " + strings.Join(parts, ", ")
}

func printJson(value interface{}) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode the answer : %s", err)
	}

	fmt.Println(string(encoded))

	return nil
}
