package wigo

import (
	"fmt"
	"encoding/json"
	"time"
)

type Notification struct {

	Type		string

	Receiver	string
	Message		string
	Date		string

	Host				*Host
	Hostname			string
	HostProbesInError	[]string

	OldProbe	*ProbeResult
	NewProbe	*ProbeResult

}

func NewNotification( t string, receiver string, host *Host, oldProbe *ProbeResult, newProbe *ProbeResult) ( this *Notification ){

	if( t != "url" ){
		return nil
	}

	this = new(Notification)
	this.Type		= t
	this.Host		= host
	this.OldProbe	= oldProbe
	this.NewProbe	= newProbe
	this.Message 	= fmt.Sprintf("Probe %s switched from %d to %d on host %s", oldProbe.Name, oldProbe.Status, newProbe.Status, host.Name)
	this.Receiver	= receiver
	this.Hostname	= host.Name

	this.HostProbesInError = host.GetErrorsProbesList()

	return
}

func (this *Notification) Send( ci chan Event ){
	this.HostProbesInError = this.Host.GetErrorsProbesList()
	this.Date = time.Now().Format(dateLayout)
	ci <- Event{ SENDNOTIFICATION, this }
}

func (this *Notification) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}
