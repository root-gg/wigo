package wigo

import (
	"log"
	"os"
	"fmt"
)


type Wigo struct {
	Version			string
	GlobalStatus	int
	Hosts			map[string] *Host

	config			*Config
}

func InitWigo( configFile string ) ( this *Wigo ){

	this 				= new(Wigo)
	this.Version 		= "Wigo v0.2"
	this.GlobalStatus	= 0
	this.Hosts			= make(map[string] *Host)

	// Private vars
	this.config			= NewConfig(configFile)


	// Create LocalHost
	localHost := NewLocalHost()
	this.Hosts[localHost.Name] = localHost

	// Init channels
	InitChannels()

	return
}

func (this *Wigo) RecomputeGlobalStatus() {

	this.GlobalStatus = 0

	for hostname := range this.Hosts {
		if (this.Hosts[hostname].Status > this.GlobalStatus) {
			this.GlobalStatus = this.Hosts[hostname].Status
		}
	}

	return
}

func (this *Wigo) GetLocalHost() ( *Host ){

	localHostname, err := os.Hostname()
	if err != nil {
		// TODO
	}

	return this.Hosts[ localHostname ]
}

func (this *Wigo) GetConfig() (*Config){
	return this.config
}

func (this *Wigo) AddHost( host *Host ){

	// Create host if not exist
	if _, ok := this.Hosts[ host.Name ] ; !ok {
		this.Hosts[ host.Name ] = host
	}

	// Update probes
	for probeName := range host.Probes{
		this.AddOrUpdateProbe(host, host.Probes[probeName])
	}
}

func (this *Wigo) AddOrUpdateProbe( host *Host, probe *ProbeResult ){
	
	// Add host it not exist
	if _, ok := this.Hosts[ host.Name ] ; !ok {
		this.Hosts[ host.Name ] = host
	}
	
	// If old prove, test if status is different
	if oldProbe, ok := this.Hosts[ host.Name ].Probes[ probe.Name ] ; ok {

		// Notification
		if oldProbe.Status != probe.Status {
			message := fmt.Sprintf("Probe %s on host %s switch from %d to %d\n", oldProbe.Name, host.Name, oldProbe.Status, probe.Status)
			log.Println(message)

			if(this.config.CallbackUrl != ""){
				notification := NewNotification("url", this.config.CallbackUrl, host, oldProbe, probe )
				notification.SendNotification( Channels.ChanCallbacks )
			}
		}
	}

	// Update
	this.Hosts[ host.Name ].Probes[ probe.Name ] = probe

	// Recompute status
	host.RecomputeStatus()
	this.RecomputeGlobalStatus()

	return
}

func (this *Wigo) MergeRemoteWigoWithLocal( remoteWigo *Wigo ) {
	for remoteHostname := range remoteWigo.Hosts{
		this.AddHost(remoteWigo.Hosts[remoteHostname])
	}
}
