package wigo

import (
	"encoding/json"
	"sync"
	"os"
	"fmt"
	"log"
	"io"

	"code.google.com/p/go-uuid/uuid"
)

// Static global object
var LocalWigo 	*Wigo

type Wigo struct {
	Uuid			string
	Version			string
	IsAlive			bool

	GlobalStatus	int
	GlobalMessage	string

	LocalHost		*Host
	RemoteWigos		map[string] *Wigo

	hostname		string
	config			*Config
	locker			*sync.RWMutex
}

func InitWigo() ( err error ){

	if LocalWigo == nil {
		LocalWigo 				= new(Wigo)

		LocalWigo.Uuid			= uuid.New()
		LocalWigo.IsAlive 		= true
		LocalWigo.Version 		= "Wigo v0.40"
		LocalWigo.GlobalStatus 	= 100
		LocalWigo.GlobalMessage = "OK"


		// Init LocalHost and RemoteWigos list
		LocalWigo.LocalHost 	= NewLocalHost()
		LocalWigo.RemoteWigos 	= make(map[string] *Wigo)

		// Private vars
		LocalWigo.config 		= NewConfig()
		LocalWigo.hostname 		= "localhost"
		LocalWigo.locker 		= new(sync.RWMutex)


		// Log
		f, err := os.OpenFile(LocalWigo.GetConfig().LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			fmt.Printf("Fail to open logfile %s : %s\n", LocalWigo.GetConfig().LogFile, err)
		} else {
			writer := io.MultiWriter( os.Stdout, f )

			log.SetOutput(writer)
			log.SetPrefix(LocalWigo.GetLocalHost().Name + " ")
		}

		// Test probes directory
		_, err = os.Stat( LocalWigo.GetConfig().ProbesDirectory )
		if err != nil {
			return err
		}

		// Init channels
		InitChannels()
	}

	return nil
}

// Factory

func GetLocalWigo() ( *Wigo ){
	return LocalWigo
}


// Constructors

func NewWigoFromJson( ba []byte ) ( this *Wigo, e error ){

	this 			= new(Wigo)
	this.IsAlive 	= true

	err := json.Unmarshal( ba, this )
	if( err != nil ){
		return nil, err
	}

	this.SetParentHostsInProbes()

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

	this.Lock()
	defer this.Unlock()

	// Test is remote is not me :D
	if remoteWigo.Uuid!= "" && this.Uuid == remoteWigo.Uuid {
		log.Printf("Try to add a remote wigo %s with same uuid as me.. Discarding..",remoteWigo.GetHostname())
		return
	}

	if oldWigo, ok := this.RemoteWigos[ wigoName ] ; ok {
		this.CompareTwoWigosAndRaiseNotifications(oldWigo,remoteWigo)
	}

	this.RemoteWigos[ wigoName ] = remoteWigo
	this.RecomputeGlobalStatus()
}


func (this *Wigo) CompareTwoWigosAndRaiseNotifications( oldWigo *Wigo, newWigo *Wigo ) (){


	// Send wigo notif if status is not the same
	if(newWigo.GlobalStatus != oldWigo.GlobalStatus){
		Channels.ChanCallbacks <- NewNotificationWigo( oldWigo, newWigo )
	}

	// Detect changes and deleted probes
	for probeName := range oldWigo.LocalHost.Probes {
		oldProbe := oldWigo.LocalHost.Probes[ probeName ]

		if probeWhichStillExistInNew, ok := newWigo.LocalHost.Probes[ probeName ] ; ok {

			// Probe still exist in new
			// Status has changed ? -> Notification
			if ( oldProbe.Status != probeWhichStillExistInNew.Status ) {
				Channels.ChanCallbacks <- NewNotificationProbe( oldProbe, probeWhichStillExistInNew )
			}
		} else {

			// Prob disappeard !
			if newWigo.IsAlive {
				Channels.ChanCallbacks <- NewNotificationProbe(oldProbe, nil)
			}
		}
	}

	// Detect new probes (only if new wigo is up)
	if newWigo.IsAlive && oldWigo.IsAlive {
		for probeName := range newWigo.LocalHost.Probes {
			if _,ok := oldWigo.LocalHost.Probes[probeName] ; !ok {
				Channels.ChanCallbacks <- NewNotificationProbe( nil, newWigo.LocalHost.Probes[probeName] )
			}
		}
	}

	// Remote Wigos
	for wigoName := range oldWigo.RemoteWigos {

		oldWigo := oldWigo.RemoteWigos[ wigoName ]

		if wigoStillExistInNew, ok := newWigo.RemoteWigos[ wigoName ]; ok {

			// Test if a remote wigo is not me
			if wigoStillExistInNew.Uuid != "" && this.Uuid == wigoStillExistInNew.Uuid {
				log.Printf("Detected myself in remote wigo %s. Discarding.", wigoStillExistInNew.GetHostname())
				return
			}

			// Recursion
			this.CompareTwoWigosAndRaiseNotifications(oldWigo, wigoStillExistInNew)
		}
	}
}


func (this *Wigo) SetParentHostsInProbes(){
	for localProbeName := range this.GetLocalHost().Probes {
		this.LocalHost.Probes[localProbeName].SetHost( this.LocalHost )
	}

	for remoteWigo := range this.RemoteWigos{
		this.RemoteWigos[remoteWigo].SetParentHostsInProbes()
	}
}


// Locks
func (this *Wigo) Lock(){
	this.locker.Lock()
}

func (this *Wigo) Unlock(){
	this.locker.Unlock()
}


// Serialize
func (this *Wigo) ToJsonString() ( string, error ){

	// Send json to socket channel
	j, e := json.MarshalIndent( this, "", "    ")
	if ( e != nil ) {
		return "", e
	}


	return string(j), nil
}
