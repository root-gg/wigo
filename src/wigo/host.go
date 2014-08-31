package wigo

import (
	"os"
	"log"
)


// Host

type Host struct {
	Name                string

	GlobalStatus        int
	Probes              map[string] *ProbeResult
}

func NewHost( hostname string ) ( this *Host ){

	this                = new( Host )

	this.GlobalStatus   = 0
	this.Name           = hostname
	this.Probes         = make(map[string] *ProbeResult)

	return
}

func NewLocalHost() ( this *Host ){

	this                = new( Host )
	this.GlobalStatus	= 0
	this.Probes         = make(map[string] *ProbeResult)

	// Get hostname
	localHostname, err := os.Hostname()
	if( err != nil ){
		log.Println("Couldn't get hostname for local machine, using localhost")

		this.Name	= "localhost"
	} else {
		this.Name	= localHostname
	}

	return
}
