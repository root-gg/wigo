package wigo


import (
	"encoding/json"
	"time"
	"strings"
	"log"
)

const dateLayout  = "2006-01-02T15:04:05.999999 (MST)"

// OpenTSDB
type Put struct {
	Value		float64
	Tags		map[string]string
}

type ProbeResult struct {

	Name        	string
	Version     	string
	Value       	interface{}
	Message     	string
	ProbeDate   	string

	Metrics    		interface{}
	Detail      	interface{}

	Status      	int
	ExitCode    	int

	parentHost	*Host
}

func NewProbeResultFromJson( name string, ba []byte ) ( this *ProbeResult ){
	this = new( ProbeResult )

	json.Unmarshal( ba, this )

	this.Name      	= name
	this.ProbeDate 	= time.Now().Format(dateLayout)
	this.ExitCode  	= 0

	this.parentHost = GetLocalWigo().GetLocalHost()

	return
}
func NewProbeResult( name string, status int, exitCode int, message string, detail string ) ( this *ProbeResult ){
	this = new( ProbeResult )

	this.Name       = name
	this.Status     = status
	this.ExitCode   = exitCode
	this.Message    = message
	this.Detail     = detail
	this.ProbeDate  = time.Now().Format(dateLayout)

	this.parentHost = GetLocalWigo().GetLocalHost()

	return
}


// Getters
func ( this *ProbeResult ) GetHost() ( *Host ){
	return this.parentHost
}


// Setters
func ( this *ProbeResult ) SetHost( h *Host )(){
	this.parentHost = h
}

func ( this *ProbeResult ) GraphMetrics(){

	if GetLocalWigo().GetConfig().OpenTSDBEnabled {
		if _, ok := this.Metrics.([]interface{}) ; ok {
			puts := this.Metrics.([]interface{})

			for i := range puts {
				if _, ok := puts[i].(map[string] interface{}) ; ok {
					put := new(Put)
					putTmp := puts[i].(map[string] interface{})

					// Test if we have value
					if _, ok := putTmp["Value"].(float64) ; ok {
						put.Value = putTmp["Value"].(float64)
					} else {
						continue
					}

					// Tags
					put.Tags = make(map[string]string)
					put.Tags["hostname"] = this.GetHost().Name

					if _, ok := putTmp["Tags"].(map[string]interface{}) ; ok {
						for k, v := range putTmp["Tags"].(map[string]interface{}) {
							if _, ok := v.(string) ; ok {
								put.Tags[strings.ToLower(k)] = string(v.(string))
							}
						}
					}

					// Push
					putStr, err := GetLocalWigo().GetOpenTsdb().Put("wigo."+this.Name, put.Value, put.Tags)
					if err != nil {
						log.Printf("Error while pushing to OpenTSDB : %s", err)
					}

					log.Printf("[TSD] " + putStr + "\n")
				}
			}
		}
	}

	return
}
