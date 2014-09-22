package wigo

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type Notification struct {
	Type    string
	Message string
	Date    string
	Summary string
}

type INotification interface {
	ToJson() ([]byte, error)
	GetMessage() string
	GetSummary() string
}

type NotificationWigo struct {
	*Notification
	OldWigo *Wigo
	NewWigo *Wigo
}
type NotificationProbe struct {
	*Notification
	OldProbe          *ProbeResult
	NewProbe          *ProbeResult
	HostProbesInError []string
}

// Constructors
func NewNotification() (this *Notification) {
	this = new(Notification)
	this.Date = time.Now().Format(dateLayout)
	return
}

func NewNotificationWigo(oldWigo *Wigo, newWigo *Wigo) (this *NotificationWigo) {
	this = new(NotificationWigo)
	this.Notification = NewNotification()
	this.Type = "Wigo"
	this.OldWigo = oldWigo
	this.NewWigo = newWigo

	if oldWigo.IsAlive && !newWigo.IsAlive {
		// UP -> DOWN
		this.Message = fmt.Sprintf("Wigo %s DOWN : %s", newWigo.GetHostname(), newWigo.GlobalMessage)

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
		weSend := false

		if newWigo.IsAlive < !oldWigo.IsAlive {
			// It's an UP
			weSend = true
		} else if !newWigo.IsAlive && oldWigo.IsAlive {
			// It's a DOWN, check if new status is > to MinLevelToSendNotifications
			weSend = true
		}

		if weSend {
			Channels.ChanCallbacks <- this
		}
	}

	return
}

func NewNotificationProbe(oldProbe *ProbeResult, newProbe *ProbeResult) (this *NotificationProbe) {
	this = new(NotificationProbe)
	this.Notification = NewNotification()
	this.Type = "Probe"
	this.OldProbe = oldProbe
	this.NewProbe = newProbe

	if oldProbe == nil && newProbe != nil {
		this.Message = fmt.Sprintf("New probe %s with status %d detected on host %s", newProbe.Name, newProbe.Status, newProbe.GetHost().Name)

		this.Summary += fmt.Sprintf("A new probe %s has been detected on host %s : \n\n", newProbe.Name, newProbe.GetHost().Name)
		this.Summary += fmt.Sprintf("\t%s\n", newProbe.Message)

	} else if oldProbe != nil && newProbe == nil {
		this.Message = fmt.Sprintf("Probe %s on host %s does not exist anymore. Last status was %d", oldProbe.Name, oldProbe.GetHost().Name, oldProbe.Status)

		this.Summary += fmt.Sprintf("Probe %s has been deleted on host %s : \n\n", oldProbe.Name, oldProbe.GetHost().Name)
		this.Summary += fmt.Sprintf("Last message was : \n\n%s\n", oldProbe.Message)

	} else if oldProbe != nil && newProbe != nil {
		if newProbe.Status != oldProbe.Status {
			this.Message = fmt.Sprintf("Probe %s status changed from %d to %d on host %s", newProbe.Name, oldProbe.Status, newProbe.Status, oldProbe.GetHost().GetParentWigo().GetHostname())

			this.Summary += fmt.Sprintf("Probe %s on host %s : \n\n", oldProbe.Name, oldProbe.GetHost().GetParentWigo().GetHostname())
			this.Summary += fmt.Sprintf("\tOld Status : %d\n", oldProbe.Status)
			this.Summary += fmt.Sprintf("\tNew Status : %d\n\n", newProbe.Status)
			this.Summary += fmt.Sprintf("Message :\n\n\t%s\n\n", newProbe.Message)

			// List parent host probes in error
			this.HostProbesInError = newProbe.parentHost.GetErrorsProbesList()
		}
	}

	// Log
	log.Printf("New Probe Notification : %s", this.Message)

	// Send ?
	if GetLocalWigo().GetConfig().NotificationsOnProbeChange {
		weSend := false

		if oldProbe != nil && newProbe != nil {
			if newProbe.Status < oldProbe.Status && oldProbe.Status >= GetLocalWigo().GetConfig().MinLevelToSendNotifications {
				// It's an UP
				weSend = true
			} else if newProbe.Status >= GetLocalWigo().GetConfig().MinLevelToSendNotifications {
				// It's a DOWN, check if new status is > to MinLevelToSendNotifications
				weSend = true
			}
		}

		if weSend {
			Channels.ChanCallbacks <- this
		}
	}

	return
}

// Getters
func (this *Notification) ToJson() (ba []byte, e error) {
	return json.Marshal(this)
}
func (this *NotificationWigo) ToJson() (ba []byte, e error) {
	return json.Marshal(this)
}
func (this *NotificationProbe) ToJson() (ba []byte, e error) {
	return json.Marshal(this)
}

func (this *Notification) GetSummary() (s string) {
	return this.Summary
}
func (this *NotificationWigo) GetSummary() (s string) {
	return this.Summary
}
func (this *NotificationProbe) GetSummary() (s string) {
	return this.Summary
}

func (this *Notification) GetMessage() string {
	return this.Message
}
