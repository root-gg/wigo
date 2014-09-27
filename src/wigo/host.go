package wigo

import (
	"log"
	"os"
)

// Host

type Host struct {
	Name       string
	Group      string
	Status     int
	Probes     map[string]*ProbeResult
	parentWigo *Wigo
}

func NewHost(hostname string) (this *Host) {

	this = new(Host)

	this.Status = 0
	this.Name = hostname
	this.Group = ""
	this.Probes = make(map[string]*ProbeResult)

	return
}

func NewLocalHost() (this *Host) {

	this = new(Host)
	this.Status = 100
	this.Probes = make(map[string]*ProbeResult)

	// Get hostname
	localHostname, err := os.Hostname()
	if err != nil {
		log.Println("Couldn't get hostname for local machine, using localhost")

		this.Name = "localhost"
	} else {
		this.Name = localHostname
	}

	// Set parent wigo
	this.parentWigo = GetLocalWigo()

	// Set group
	this.Group = GetLocalWigo().GetConfig().Global.Group

	return
}

// Methods

func (this *Host) RecomputeStatus() {

	this.Status = 0

	for probeName := range this.Probes {
		if this.Probes[probeName].Status > this.Status {
			this.Status = this.Probes[probeName].Status
		}
	}

	return
}

func (this *Host) AddOrUpdateProbe(probe *ProbeResult) {

    oldWigoJson, _  := GetLocalWigo().ToJsonString()
    oldWigo, _      := NewWigoFromJson([]byte(oldWigoJson), 0)

	// If old probe, test if status is different
	if oldProbe, ok := GetLocalWigo().GetLocalHost().Probes[probe.Name]; ok {

		// Notification
		if oldProbe.Status != probe.Status {
			NewNotificationProbe(oldProbe, probe)
		}
	} else {

		// New probe
		probe.SetHost(this)
	}

	// Update
	GetLocalWigo().LocalHost.Probes[probe.Name] = probe
	GetLocalWigo().LocalHost.RecomputeStatus()

	// Graph
	probe.GraphMetrics()

	// Recompute status
	GetLocalWigo().RecomputeGlobalStatus()

    // Raise wigo notification if status changed
    if GetLocalWigo().GlobalStatus != oldWigo.GlobalStatus {
        NewNotificationWigo(oldWigo,GetLocalWigo())
    }

	return
}

func (this *Host) DeleteProbeByName(probeName string) {
	if probeToDelete, ok := this.Probes[probeName]; ok {
		NewNotificationProbe(probeToDelete, nil)
		delete(this.Probes, probeName)
	}
}

func (this *Host) GetErrorsProbesList() (list []string) {

	list = make([]string, 0)

	for probeName := range this.Probes {
		if this.Probes[probeName].Status > 100 {
			list = append(list, probeName)
		}
	}

	return
}

func (this *Host) GetParentWigo() *Wigo {
	return this.parentWigo
}
func (this *Host) SetParentWigo(w *Wigo) {
	this.parentWigo = w
}
