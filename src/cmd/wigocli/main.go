package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
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
	wigocli [--config=CONFIG]
	wigocli [--config=CONFIG] detail
	wigocli [--config=CONFIG] probe <probe>
	wigocli [--config=CONFIG] remote <wigo>
	wigocli [--config=CONFIG] remote <wigo> detail
	wigocli [--config=CONFIG] remote <wigo> probe <probe>
	wigocli -h | --help
	wigocli --version

Commands:
	detail

Options
	-h 	--help              Show help
	--version               Show version
	--config=CONFIG		Specify config file
`, wigo.Version)

	// Parse args
	opts, _ := docopt.ParseArgs(usage, os.Args[1:], wigo.Version)

	configFile, _ := opts.String("--config")
	if configFile == "" {
		configFile = "/etc/wigo/wigo.conf"
	}
	showDetails, _ := opts.Bool("detail")
	showOnlyErrors = !showDetails
	probe, _ := opts.String("<probe>")
	wigoHostname, _ := opts.String("<wigo>")
	if wigoHostname != "" {
		wigoHost = wigoHostname
	}

	config := wigo.NewConfig(configFile)
	if !config.Http.Enabled {
		fmt.Printf("HTTP server is not enabled in config file\n")
		os.Exit(1)
	}

	httpAddress := config.Http.Address
	if httpAddress == "0.0.0.0" {
		httpAddress = "127.0.0.1"
	}
	httpPort := config.Http.Port
	httpSslEnabled := config.Http.SslEnabled
	httpProtocol := "http"
	if httpSslEnabled {
		httpProtocol = "https"
	}
	httpLogin := config.Http.Login
	httpPassword := config.Http.Password

	apiUrl := fmt.Sprintf("%s://%s:%d/api", httpProtocol, httpAddress, httpPort)
	if httpLogin != "" && httpPassword != "" {
		apiUrl = fmt.Sprintf("%s://%s:%s@%s:%d/api", httpProtocol, httpLogin, httpPassword, httpAddress, httpPort)
	}

	// Create http client
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Connect
	resp, err := httpClient.Get(apiUrl)
	if err != nil {
		fmt.Printf("Error : %s\n", err)
		os.Exit(1)
	}
	body, err := io.ReadAll(resp.Body)

	// Instanciate object from json
	wigoObj, err := wigo.NewWigoFromJson(body, 0)
	if err != nil {
		fmt.Printf("Failed to parse return from host : %s\n", err)
	}

	// Print summary
	if probe != "" {
		if wigoHost == "localhost" {
			// Find probe
			if tmp, ok := wigoObj.GetLocalHost().Probes.Get(probe); ok {
				p := tmp.(*wigo.ProbeResult)
				fmt.Println(p.Summary())
			} else {
				fmt.Printf("Probe %s not found in local wigo\n", probe)
			}
		} else {
			// Find wigo

			if tmp, ok := wigoObj.RemoteWigos.Get(wigoHost); ok {
				w := tmp.(*wigo.Wigo)
				// Find probe
				if tmp, ok := w.GetLocalHost().Probes.Get(probe); ok {
					p := tmp.(*wigo.ProbeResult)
					fmt.Println(p.Summary())
				} else {
					fmt.Printf("Probe %s not found on remote wigo %s\n", probe, wigoHost)
				}
			} else {
				fmt.Printf("Remote wigo %s not found\n", wigoHost)
			}
		}
	} else if wigoHost != "" && wigoHost != "localhost" {
		// Find remote
		if tmp, ok := wigoObj.RemoteWigos.Get(wigoHost); ok {
			w := tmp.(*wigo.Wigo)
			fmt.Print(w.GenerateSummary(showOnlyErrors))
		} else {
			fmt.Printf("Remote wigo %s not found\n", wigoHost)
		}
	} else {
		fmt.Print(wigoObj.GenerateSummary(showOnlyErrors))
	}
}
