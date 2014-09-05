
package  main

import (
	"net"
	"time"
	"wigo"
	"bytes"
	"fmt"
)


func main(){

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
	fmt.Printf(wigoObj.GenerateSummary(true))

}
