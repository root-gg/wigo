package wigo

import (
	"os"
	"log"
)


// Host

type Host struct {
	Name                string

	Status        		int
	Probes              map[string] *ProbeResult
}

func NewHost( hostname string ) ( this *Host ){

	this                = new( Host )

	this.Status   		= 0
	this.Name           = hostname
	this.Probes         = make(map[string] *ProbeResult)

	return
}

func NewLocalHost() ( this *Host ){

	this                = new( Host )
	this.Status			= 0
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



// Methods

func (this *Host) RecomputeStatus(){

	this.Status = 0

	for probeName := range this.Probes {
		if(this.Probes[probeName].Status > this.Status){
			this.Status = this.Probes[probeName].Status
		}
	}

	return
}

func (this *Host) GetErrorsProbesList() ( list []string ){

	list = make([]string,0)

	for probeName := range this.Probes {
		if this.Probes[probeName].Status > 100 {
			list = append(list, probeName)
		}
	}

	return
}


