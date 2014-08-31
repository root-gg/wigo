package wigo

import (
	"fmt"
	"encoding/json"
)


// Misc
func Dump(data interface{}) {
	json, _ := json.MarshalIndent(data, "", "   ")
	fmt.Printf("%s\n", string(json))
}
