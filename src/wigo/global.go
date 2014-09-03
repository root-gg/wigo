package wigo

import (
	"log"
	"encoding/json"
)


type Wigo struct {
	Version			string
	IsAlive			bool

	GlobalStatus	int
	GlobalMessage	string

	LocalHost		*Host
	RemoteWigos		map[string] *Wigo

	config			*Config
	hostname		string
}

func InitWigo( configFile string ) ( this *Wigo ){

	this 				= new(Wigo)

	this.IsAlive		= true
	this.Version 		= "Wigo v0.32"
	this.GlobalStatus	= 0
	this.GlobalMessage	= "OK"

	this.LocalHost		= NewLocalHost()
	this.RemoteWigos	= make(map[string] *Wigo)

	// Private vars
	this.config			= NewConfig(configFile)
	this.hostname		= "localhost"

	// Init channels
	InitChannels()

	return
}

// Constructors

func NewWigoFromJson( ba []byte ) ( this *Wigo, e error ){

	this 			= new(Wigo)
	this.IsAlive 	= true

	err := json.Unmarshal( ba, this )
	if( err != nil ){
		return nil, err
	}

	return
}

func NewWigoFromErrorMessage( message string, isAlive bool ) ( this *Wigo ){

	this = new(Wigo)
	this.GlobalStatus 	= 500
	this.GlobalMessage	= message
	this.IsAlive		= isAlive
	this.RemoteWigos	= make(map[string] *Wigo)
	this.LocalHost		= new(Host)

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

	if oldWigo, ok := this.RemoteWigos[ wigoName ] ; ok {
		this.CompareTwoWigosAndRaiseNotifications(oldWigo,remoteWigo)
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

			if (this.config.CallbackUrl != "") {
				Channels.ChanCallbacks <- NewNotificationProbe( this.config.CallbackUrl, oldProbe, probe )
			}
		}
	}

	// Update
	this.LocalHost.Probes[ probe.Name ] = probe
	this.LocalHost.RecomputeStatus()

	// Recompute status
	this.RecomputeGlobalStatus()

	return
}


func (this *Wigo) CompareTwoWigosAndRaiseNotifications( oldWigo *Wigo, newWigo *Wigo ) (){


	// Send wigo notif if status is not the same
	if(newWigo.GlobalStatus != oldWigo.GlobalStatus){
		Channels.ChanCallbacks <- NewNotificationWigo( this.config.CallbackUrl, oldWigo, newWigo)
	}

	// LocalProbes
	for probeName := range oldWigo.LocalHost.Probes {
		oldProbe := oldWigo.LocalHost.Probes[ probeName ]

		if probeWhichStillExistInNew, ok := newWigo.LocalHost.Probes[ probeName ] ; ok {

			// Probe still exist in new
			// Status has changed ? -> Notification
			if ( oldProbe.Status != probeWhichStillExistInNew.Status ) {
				Channels.ChanCallbacks <- NewNotificationProbe( this.config.CallbackUrl, oldProbe, probeWhichStillExistInNew)
			}

		} else {
			// New Probe
		}
	}


	// Remote Wigos
	for wigoName := range oldWigo.RemoteWigos {

		oldWigo := oldWigo.RemoteWigos[ wigoName ]

		if wigoStillExistInNew, ok := newWigo.RemoteWigos[ wigoName ]; ok {
			this.CompareTwoWigosAndRaiseNotifications(oldWigo, wigoStillExistInNew)
		}
	}
}
