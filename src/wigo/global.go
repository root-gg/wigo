package wigo

import (
	"log"
	"container/list"
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

	if oldWigo, ok := this.RemoteWigos[ wigoName ] ; ok {
		notifications := this.CompareTwoWigosAndRaiseNotifications(oldWigo,remoteWigo)

		if notifications.Len() > 0 {
			log.Printf("There is some notifications from remote wigo %s : \n", wigoName)

			for e := notifications.Front(); e != nil; e = e.Next() {
				log.Printf(" - %s\n", e.Value.(*Notification).Message)
			}
		}
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


func (this *Wigo) CompareTwoWigosAndRaiseNotifications( oldWigo *Wigo, newWigo *Wigo ) ( allNotifications *list.List ){

	allNotifications = list.New()

	// LocalProbes
	for probeName := range oldWigo.LocalHost.Probes{
		oldProbe := oldWigo.LocalHost.Probes[ probeName ]

		if probeWhichStillExistInNew, ok := newWigo.LocalHost.Probes[ probeName ] ; ok {

			// Probe still exist in new
			// Status has changed ? -> Notification
			if( oldProbe.Status != probeWhichStillExistInNew.Status ){
				notification := NewNotification("url", this.config.CallbackUrl, oldWigo.LocalHost, oldProbe, probeWhichStillExistInNew )
				allNotifications.PushBack(notification)
			}

		} else {
			// New Probe
		}
	}


	// Remote Wigos
	for wigoName := range oldWigo.RemoteWigos {

		oldWigo := oldWigo.RemoteWigos[ wigoName ]

		if wigoStillExistInNew, ok := newWigo.RemoteWigos[ wigoName ]; ok {
			remoteNotifications := this.CompareTwoWigosAndRaiseNotifications( oldWigo, wigoStillExistInNew )
			allNotifications.PushBackList(remoteNotifications)
		}
	}

	return allNotifications
}
