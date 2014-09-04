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


func (this *Host) AddOrUpdateProbe( probe *ProbeResult ){

	// If old prove, test if status is different
	if oldProbe, ok := GetLocalWigo().GetLocalHost().Probes[ probe.Name ] ; ok {

		// Notification
		if oldProbe.Status != probe.Status {
			log.Printf("Probe %s on host %s switch from %d to %d\n", oldProbe.Name, GetLocalWigo().GetLocalHost().Name, oldProbe.Status, probe.Status)

			Channels.ChanCallbacks <- NewNotificationProbe( oldProbe, probe )
		}
	} else {

		// New probe
		probe.SetHost( this )
	}

	// Update
	GetLocalWigo().LocalHost.Probes[ probe.Name ] = probe
	GetLocalWigo().LocalHost.RecomputeStatus()

	// Recompute status
	GetLocalWigo().RecomputeGlobalStatus()

	return
}


func (this *Host) DeleteProbeByName( probeName string ){
	if probeToDelete, ok := this.Probes[ probeName ] ; ok {
		Channels.ChanCallbacks <- NewNotificationProbe( probeToDelete, nil )
		delete(this.Probes,probeName)
	}
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


