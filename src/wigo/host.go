package wigo

// Host

type Host struct {
	Name       string
	Group      string
	Status     int
	Probes     *concurrentMapProbes
	parentWigo *Wigo
}

type HostSummary struct {
	Name    string
	Message string
	Status  int
	IsAlive bool
	Probes  []map[string]interface{}
}

func NewHost() (this *Host) {

	this = new(Host)

	this.Status = 100
	this.Probes = NewConcurrentMapProbes()

	return
}

// Methods

func (this *Host) RecomputeStatus() {

	this.Status = 0

	for item := range this.Probes.IterBuffered() {
		probe := item.Val.(*ProbeResult)

		if probe.Status > this.Status {
			this.Status = probe.Status
		}
	}

	return
}

func (this *Host) AddOrUpdateProbe(probe *ProbeResult) {

	// If old probe, test if status is different
	if tmp, ok := GetLocalWigo().GetLocalHost().Probes.Get(probe.Name); ok {
		oldProbe := tmp.(*ProbeResult)

		// Notification
		if oldProbe.Status != probe.Status {
			NewNotificationProbe(oldProbe, probe)
		}
	} else {

		// New probe
		probe.SetHost(this)
	}

	// Update
	GetLocalWigo().LocalHost.Probes.Set(probe.Name, probe)
	GetLocalWigo().LocalHost.RecomputeStatus()

	// Graph
	probe.GraphMetrics()

	// Recompute status
	GetLocalWigo().RecomputeGlobalStatus()

	return
}

func (this *Host) DeleteProbeByName(probeName string) {
	if tmp, ok := this.Probes.Get(probeName); ok {
		probeToDelete := tmp.(*ProbeResult)
		NewNotificationProbe(probeToDelete, nil)
		this.Probes.Remove(probeName)
	}
}

func (this *Host) GetErrorsProbesList() (list []string) {

	list = make([]string, 0)

	for item := range this.Probes.IterBuffered() {
		probeName := item.Key
		probe := item.Val.(*ProbeResult)

		if probe.Status > 100 {
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

func (this *Host) GetSummary() (hs *HostSummary) {
	hs = new(HostSummary)
	hs.Name = this.Name
	hs.Status = this.Status
	hs.Probes = make([]map[string]interface{}, 0)

	for item := range this.Probes.IterBuffered() {
		_probe := item.Val.(*ProbeResult)

		probe := make(map[string]interface{})
		probe["Name"] = _probe.Name
		probe["Status"] = _probe.Status
		probe["Message"] = _probe.Message

		hs.Probes = append(hs.Probes, probe)
	}

	return hs
}
