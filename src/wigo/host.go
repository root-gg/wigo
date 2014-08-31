package wigo


// Host

type Host struct {
	Name                string

	GlobalStatus        int
	Probes              map[string] *ProbeResult
}

func NewHost( hostname string ) ( this *Host ){

	this                = new( Host )

	this.GlobalStatus   = 0
	this.Name           = hostname
	this.Probes         = make(map[string] *ProbeResult)

	return
}
