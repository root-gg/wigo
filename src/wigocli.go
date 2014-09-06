
package  main

import (
	"net"
	"time"
	"wigo"
	"bytes"
	"fmt"

	"github.com/docopt/docopt-go"
	"os"
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
	conn, err := net.DialTimeout("tcp", "127.0.0.1:4000", time.Second * 2)
	if err != nil {
		fmt.Printf("Could not connect to wigo : %s", err)
		return
	}


	// Get content
	completeOutput := new(bytes.Buffer)

	for {
		reply := make([]byte, 512)
		read_len, err := conn.Read(reply)
		if ( err != nil ) {
			break
		}

		completeOutput.Write(reply[:read_len])
	}

	// Instanciate object from json
	wigoObj, err := wigo.NewWigoFromJson(completeOutput.Bytes())
	if (err != nil) {
		fmt.Printf("Failed to parse return from host : %s", err)
	}


	// Print summary
	fmt.Printf(wigoObj.GenerateSummary(showOnlyErrors))

}
