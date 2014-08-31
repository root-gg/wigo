package wigo

type Wigo struct {

	Version			string
	GlobalStatus	int
	Hosts			map[string] *Host

}

func InitWigo() ( this *Wigo ){

	this 				= new(Wigo)

	this.Version 		= "Wigo v0.1"
	this.GlobalStatus	= 0
	this.Hosts			= make(map[string] *Host)

	return
}

func (this *Wigo) RecomputeGlobalStatus() {

	this.GlobalStatus = 0

	for hostname := range this.Hosts {
		if (this.Hosts[hostname].Status > this.GlobalStatus) {
			this.GlobalStatus = this.Hosts[hostname].Status
		}
	}

	return
}
