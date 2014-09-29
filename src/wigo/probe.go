package wigo

import (
	"encoding/json"
	"github.com/bodji/gopentsdb"
	"log"
	"strings"
	"time"
	"strconv"
	"github.com/fatih/color"
)

const dateLayout = "2006-01-02T15:04:05.999999 (MST)"

// OpenTSDB
type Put struct {
	Value float64
	Tags  map[string]string
}

type ProbeResult struct {
	Name      string
	Version   string
	Value     interface{}
	Message   string
	ProbeDate string

	Metrics interface{}
	Detail  interface{}

	Status   int
	ExitCode int

	parentHost *Host
}

func NewProbeResultFromJson(name string, ba []byte) (this *ProbeResult) {
	this = new(ProbeResult)

	json.Unmarshal(ba, this)

	this.Name = name
	this.ProbeDate = time.Now().Format(dateLayout)
	this.ExitCode = 0

	this.parentHost = GetLocalWigo().GetLocalHost()

	return
}
func NewProbeResult(name string, status int, exitCode int, message string, detail string) (this *ProbeResult) {
	this = new(ProbeResult)

	this.Name = name
	this.Status = status
	this.ExitCode = exitCode
	this.Message = message
	this.Detail = detail
	this.ProbeDate = time.Now().Format(dateLayout)

	this.parentHost = GetLocalWigo().GetLocalHost()

	return
}

// Getters
func (this *ProbeResult) GetHost() *Host {
	return this.parentHost
}

// Setters
func (this *ProbeResult) SetHost(h *Host) {
	this.parentHost = h
}

func (this *ProbeResult) GraphMetrics() {

	if GetLocalWigo().GetConfig().OpenTSDB.Enabled {
		if puts, ok := this.Metrics.([]interface{}); ok {
			for i := range puts {
				if putTmp, ok := puts[i].(map[string]interface{}); ok {

					// Test if we have value
					var putValue float64
					if _, ok := putTmp["Value"].(float64); ok {
						putValue = putTmp["Value"].(float64)
					} else {
						continue
					}

					// Tags
					putTags := make(map[string]string)
					putTags["hostname"] = this.GetHost().GetParentWigo().GetHostname()

					// Group ?
					if this.GetHost().Group != "" {
						putTags["group"] = this.GetHost().Group
					}

					if tags, ok := putTmp["Tags"].(map[string]interface{}); ok {
						for k, v := range tags {
							if _, ok := v.(string); ok {
								putTags[strings.ToLower(k)] = string(v.(string))
							}
						}
					}

					// Push
					put := gopentsdb.NewPut(GetLocalWigo().GetConfig().OpenTSDB.MetricPrefix+"."+this.Name, putTags, putValue)
					_, err := GetLocalWigo().GetOpenTsdb().Put(put)
					if err != nil {
						log.Printf("Error while pushing to OpenTSDB : %s", err)
					}
				}
			}
		}
	}

	return
}


func (this *ProbeResult) Summary() string {

	red := color.New(color.FgRed).SprintfFunc()
	yellow := color.New(color.FgYellow).SprintfFunc()
	green := color.New(color.FgGreen).SprintfFunc()

	// Name
	summary := "Probe " + this.Name + " : \n\n"

	// Print status
	summary += " Status 	: "
	if this.Status >= 300 {
		summary += red(strconv.Itoa(this.Status))+"\n"
	} else if this.Status >= 200 {
		summary += yellow(strconv.Itoa(this.Status))+"\n"
	} else {
		summary += green(strconv.Itoa(this.Status))+"\n"
	}

	// Message
	summary += " Message 		: " + this.Message 	+ "\n"
	summary += " Last execution	: " + this.ProbeDate + "\n\n"

	// Detail
	if this.Detail != nil {
		summary += " Detail : \n"
		summary += ToJson(this.Detail) + "\n\n"
	}

	return summary
}
