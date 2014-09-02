package wigo

import (
	"log"
)


type Wigo struct {
	Version			string
	GlobalStatus	int

	LocalHost		*Host
	RemoteWigos		map[string] *Wigo

	config			*Config
	hostname		string
}

func InitWigo( configFile string ) ( this *Wigo ){

	this 				= new(Wigo)
	this.Version 		= "Wigo v0.2"
	this.GlobalStatus	= 0

	this.LocalHost		= NewLocalHost()
	this.RemoteWigos	= make(map[string] *Wigo)

	// Private vars
	this.config			= NewConfig(configFile)
	this.hostname		= "localhost"

	// Init channels
	InitChannels()

	return
}


// Recompute statuses
func (this *Wigo) RecomputeGlobalStatus() {

	this.GlobalStatus = 0

	// Local probes
	for probeName := range this.LocalHost.Probes {
		if this.LocalHost.Probes[probeName].Status > this.GlobalStatus {
			this.GlobalStatus = this.LocalHost.Probes[probeName].Status
		}
	}

	// Remote wigos statuses
	for wigoName := range this.RemoteWigos {
		if this.RemoteWigos[wigoName].GlobalStatus > this.GlobalStatus {
			this.GlobalStatus = this.RemoteWigos[wigoName].GlobalStatus
		}
	}

	return
}



// Getters
func (this *Wigo) GetLocalHost() ( *Host ){
	return this.LocalHost
}

func (this *Wigo) GetConfig() (*Config){
	return this.config
}

func (this *Wigo) GetHostname() ( string ){
	return this.hostname
}


// Setters

func (this *Wigo) SetHostname( hostname string ){
	this.hostname = hostname
}

func (this *Wigo) AddOrUpdateRemoteWigo( wigoName string, remoteWigo * Wigo ){

	if _, ok := this.RemoteWigos[ wigoName ] ; ok {
		// It already exists
		// TEST changes


	}

	this.RemoteWigos[ wigoName ] = remoteWigo
	this.RecomputeGlobalStatus()
}


func (this *Wigo) AddOrUpdateLocalProbe( probe *ProbeResult ){
	
	// If old prove, test if status is different
	if oldProbe, ok := this.LocalHost.Probes[ probe.Name ] ; ok {

		// Notification
		if oldProbe.Status != probe.Status {
			log.Printf("Probe %s on host %s switch from %d to %d\n", oldProbe.Name, this.LocalHost.Name, oldProbe.Status, probe.Status)

			if(this.config.CallbackUrl != ""){
				notification := NewNotification("url", this.config.CallbackUrl, this.LocalHost, oldProbe, probe )
				notification.SendNotification( Channels.ChanCallbacks )
			}
		}
	}

	// Update
	this.LocalHost.Probes[ probe.Name ] = probe

	// Recompute status
	this.RecomputeGlobalStatus()

	return
}

