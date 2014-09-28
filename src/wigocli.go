package main

import (
	"fmt"
	"github.com/docopt/docopt-go"
	"io/ioutil"
	"net/http"
	"os"
	"wigo"
)

var command string 		= ""
var probe string 		= ""
var wigoHost string 	= "localhost"
var showOnlyErrors bool = true

func main() {

	// Usage
	usage := `wigocli

Usage:
	wigocli
	wigocli <command>
	wigocli probe <probe>
	wigocli remote <wigo>
	wigocli remote <wigo> probe <probe>

Commands:
	detail

Options
	--help
	--version
`

	// Parse args
	arguments, _ := docopt.Parse(usage, nil, true, "wigocli v0.2", false)

	for key, value := range arguments {

		if _, ok := value.(string); ok {
			if key == "<command>" {
				command = value.(string)

				if command == "detail" {
					showOnlyErrors = false

				} else {
					fmt.Printf("Unknown command %s\n", command)
					os.Exit(1)
				}
			} else if key == "<probe>" {
				probe = value.(string)
			} else if key == "<wigo>" {
				wigoHost = value.(string)
			}
		}
	}

	// Connect
	resp, err := http.Get("http://127.0.0.1:4000")
	if err != nil {
		fmt.Printf("Error : %s\n", err)
		os.Exit(1)
	}
	body, err := ioutil.ReadAll(resp.Body)

	// Instanciate object from json
	wigoObj, err := wigo.NewWigoFromJson(body, 0)
	if err != nil {
		fmt.Printf("Failed to parse return from host : %s\n", err)
	}

	// Print summary
	if probe != "" {
		if wigoHost == "localhost" {
			// Find probe
			if p, ok := wigoObj.GetLocalHost().Probes[probe] ; ok {
				fmt.Printf(p.Summary())
			} else {
				fmt.Printf("Probe %s not found in local wigo\n", probe)
			}
		} else {
			// Find wigo
			if w, ok := wigoObj.RemoteWigos[wigoHost] ; ok {
				// Find probe
				if p, ok := w.GetLocalHost().Probes[probe] ; ok {
					fmt.Printf(p.Summary())
				} else {
					fmt.Printf("Probe %s not found on remote wigo %s\n", probe, wigoHost)
				}
			} else {
				fmt.Printf("Remote wigo %s not found\n", wigoHost)
			}
		}
	} else if wigoHost != "" && wigoHost != "localhost" {
		// Find remote
		if w, ok := wigoObj.RemoteWigos[wigoHost] ; ok {
			fmt.Printf(w.GenerateSummary(showOnlyErrors))
		} else {
			fmt.Printf("Remote wigo %s not found\n", wigoHost)
		}
	} else {
		fmt.Printf(wigoObj.GenerateSummary(showOnlyErrors))
	}
}
