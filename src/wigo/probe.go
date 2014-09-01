package wigo


import (
	"encoding/json"
	"time"
)

const dateLayout  = "2006-01-02T15:04:05.999999 (MST)"



type ProbeResult struct {

	Name        string
	Version     string
	Value       interface{}
	Message     string
	ProbeDate   string

	Metrics     map[string]float64
	Detail      interface{}

	Status      int
	ExitCode    int
}

func NewProbeResultFromJson( name string, ba []byte ) ( this *ProbeResult ){
	this = new( ProbeResult )

	json.Unmarshal( ba, this )

	this.Name      	= name
	this.ProbeDate 	= time.Now().Format(dateLayout)
	this.ExitCode  	= 0

	return
}
func NewProbeResult( name string, status int, exitCode int, message string, detail string ) ( this *ProbeResult ){
	this = new( ProbeResult )

	this.Name       = name
	this.Status     = status
	this.ExitCode   = exitCode
	this.Message    = message
	this.Detail     = detail
	this.ProbeDate  = time.Now().Format(dateLayout)

	return
}

