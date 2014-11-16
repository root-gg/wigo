package wigo

// Host

type Host struct {
	Name       string
	Group      string
	Status     int
	Probes     map[string]*ProbeResult
	parentWigo *Wigo
}

type HostSummary struct {
	Name				string
	Message				string
	Status				int
	IsAlive				bool
	Probes				[]map[string]interface {}
}

func NewHost() (this *Host) {

	this = new(Host)

	this.Status = 100
	this.Probes = make(map[string]*ProbeResult)

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


func (this *Host) GetSummary()( hs *HostSummary ){
	hs 					= new(HostSummary)
	hs.Name 			= this.Name
	hs.Status 			= this.Status
	hs.Probes 			= make([]map[string] interface {},0)

	for probeName := range this.Probes {

		probe := make(map[string] interface {})
		probe["Name"] = this.Probes[probeName].Name
		probe["Status"] = this.Probes[probeName].Status
		probe["Message"] = this.Probes[probeName].Message

		hs.Probes = append( hs.Probes, probe )
	}

	return hs
}
