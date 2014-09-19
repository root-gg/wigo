
package  main

import (
	"wigo"
	"fmt"
	"github.com/docopt/docopt-go"
	"os"
	"net/http"
	"io/ioutil"
)

var command 		string	= ""
var showOnlyErrors 	bool 	= true

func main(){

	// Usage
	usage := `wigocli

Usage:
	wigocli
	wigocli <command>

Commands:
	detail

Options
	--help
	--version
`

	// Parse args
	arguments, _ := docopt.Parse(usage, nil, true, "wigocli v0.1", false)

	for key, value := range arguments {

		if _, ok := value.(string); ok {
			if key == "<command>" {
				command = value.(string)

				if command == "detail" {
					showOnlyErrors = false

				} else {
					fmt.Printf("Unknown command %s\n",command)
					os.Exit(1)
				}
			}
		}
	}

	// Connect
	resp, err := http.Get("http://127.0.0.1:4000")
	if err != nil {
		fmt.Printf("Error : %s\n",err)
		os.Exit(1)
	}
	body, err := ioutil.ReadAll(resp.Body)

	// Instanciate object from json
	wigoObj, err := wigo.NewWigoFromJson(body, true)
	if (err != nil) {
		fmt.Printf("Failed to parse return from host : %s", err)
	}

	// Print summary
	fmt.Printf(wigoObj.GenerateSummary(showOnlyErrors))
}
