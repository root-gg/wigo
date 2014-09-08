package wigo

import (
	"fmt"
	"encoding/json"
	"time"
	"log"
)

type Notification struct {
	Type		string
	Receiver	string
	Message		string
	Date		string
	Summary		string
}

type INotification interface {
	ToJson() 		([]byte, error)
	GetMessage()	string
	GetSummary()	string
}

type NotificationWigo struct {
	*Notification
	OldWigo		*Wigo
	NewWigo		*Wigo
}
type NotificationHost struct {
	*Notification
	OldHost		*Host
	NewHost		*Host
}
type NotificationProbe struct {
	*Notification
	OldProbe	*ProbeResult
	NewProbe	*ProbeResult
}


// Constructors
func NewNotification() ( this *Notification ){
	this				= new(Notification)
	this.Date			= time.Now().Format(dateLayout)
	return
}

func NewNotificationWigo( oldWigo *Wigo, newWigo *Wigo ) ( this *NotificationWigo ){
	this 				= new(NotificationWigo)
	this.Notification	= NewNotification()
	this.Type			= "Wigo"
	this.OldWigo 		= oldWigo
	this.NewWigo		= newWigo

	if oldWigo.IsAlive && !newWigo.IsAlive {
		// UP -> DOWN
		this.Message = fmt.Sprintf("Wigo %s DOWN : %s", newWigo.GetHostname(), newWigo.GlobalMessage )

	} else if !oldWigo.IsAlive && newWigo.IsAlive {
		// DOWN -> UP
		this.Message = fmt.Sprintf("Wigo %s UP", newWigo.GetHostname())

	} else if newWigo.GlobalStatus != oldWigo.GlobalStatus {
		// CHANGED STATUS
		this.Message = fmt.Sprintf("Wigo %s status switched from %d to %d", newWigo.GetHostname(), oldWigo.GlobalStatus, newWigo.GlobalStatus)

	}

	// Log
	log.Printf("New Wigo Notification : %s", this.Message)

	// Send ?
	if GetLocalWigo().GetConfig().NotificationsOnWigoChange {
		Channels.ChanCallbacks <- this
	}

	return
}

func NewNotificationHost( oldHost *Host, newHost *Host ) ( this *NotificationHost ){
	this 				= new(NotificationHost)
	this.Notification	= NewNotification()
	this.Type			= "Host"
	this.OldHost		= oldHost
	this.NewHost		= newHost

	if newHost.Status != oldHost.Status {
		this.Message = fmt.Sprintf("Host %s changed status from %d to %d", oldHost.Name, oldHost.Status, newHost.Status)
	}

	// Log
	log.Printf("New Host Notification : %s", this.Message)

	// Send ?
	if GetLocalWigo().GetConfig().NotificationsOnHostChange {
		Channels.ChanCallbacks <- this
	}

	return
}

func NewNotificationProbe( oldProbe *ProbeResult, newProbe *ProbeResult ) ( this *NotificationProbe ){
	this 				= new(NotificationProbe)
	this.Notification	= NewNotification()
	this.Type			= "Probe"
	this.OldProbe		= oldProbe
	this.NewProbe		= newProbe

	if oldProbe == nil && newProbe != nil {
		this.Message = fmt.Sprintf("New probe %s with status %d detected on host %s", newProbe.Name, newProbe.Status, newProbe.GetHost().Name)

	} else if oldProbe != nil && newProbe == nil {
		this.Message = fmt.Sprintf("Probe %s on host %s does not exist anymore. Last status was %d", oldProbe.Name, oldProbe.GetHost().Name, oldProbe.Status )

	} else if oldProbe != nil && newProbe != nil {
		if newProbe.Status != oldProbe.Status {
			this.Message = fmt.Sprintf("Probe %s status changed from %d to %d on host %s", newProbe.Name, oldProbe.Status, newProbe.Status, oldProbe.GetHost().Name)
		}
	}

	// Summary
	this.Summary += fmt.Sprintf("Probe %s on host %s : \n\n", oldProbe.Name, oldProbe.GetHost().Name)
	this.Summary += fmt.Sprintf("\tOld Status : %d\n",oldProbe.Status)
	this.Summary += fmt.Sprintf("\tNew Status : %d\n\n",newProbe.Status)

	if newProbe.Message != "" {
		this.Summary += fmt.Sprintf("Message :\n\n\t%s\n\n", newProbe.Message)
	}

	// Log
	log.Printf("New Probe Notification : %s", this.Message)

	// Send ?
	if GetLocalWigo().GetConfig().NotificationsOnProbeChange {
		Channels.ChanCallbacks <- this
	}

	return
}


// Getters
func (this *Notification) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}
func (this *NotificationWigo) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}
func (this *NotificationHost) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}
func (this *NotificationProbe) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}


func (this *Notification) GetSummary() ( s string ) {
	return this.Summary
}
func (this *NotificationWigo) GetSummary() ( s string ) {
	return this.Summary
}
func (this *NotificationHost) GetSummary() ( s string ) {
	return this.Summary
}
func (this *NotificationProbe) GetSummary() ( s string ) {
	return this.Summary
}


func (this *Notification) GetMessage() ( string ){
	return this.Message
}


