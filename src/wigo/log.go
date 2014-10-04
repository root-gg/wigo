package wigo

import (
	"time"
	"os"
	"log"
	"encoding/json"
	"bufio"
	"io"
)

var logFilehandle *os.File

type Log struct {
	Date		string
	Timestamp	int64
	Level		uint8
	Message 	string
	Host		string
	Probe   	string
	Group		string
}

func NewLog( level uint8, message string ) ( this *Log ){
	this 			= new(Log)
	this.Date 		= time.Now().Format(dateLayout)
	this.Timestamp	= time.Now().Unix()
	this.Level  	= level
	this.Message	= message
	this.Host		= GetLocalWigo().GetLocalHost().Name
	this.Group		= GetLocalWigo().GetLocalHost().Group
	this.Probe		= ""

	return
}

// Setters
func ( this *Log ) SetHost( hostname string ){
	this.Host = hostname
}
func ( this *Log ) SetProbe( probename string ){
	this.Host = probename
}
func ( this *Log ) SetGroup( group string ){
	this.Group = group
}

// Persist on disk
func ( this *Log ) Persist() {

	LocalWigo.logsFileLock.Lock()
	defer LocalWigo.logsFileLock.Unlock()

	if logFilehandle == nil {
		f, err := os.OpenFile(LocalWigo.GetConfig().Global.EventLog, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Printf("Critical : Failed to open events log file : %s", err)
			return
		}

		logFilehandle = f
	}

	// Json
	j, err := json.Marshal(this)
	if err != nil {
		log.Printf("CRITICAL: Failed convert log to json: %s", err)
		return
	}

	// Persist
	logFilehandle.WriteString(string(j) + "\n")

	return
}

// LoadFromDisk
func LoadLogsFromDisk(){

	LocalWigo.logsFileLock.Lock()
	defer LocalWigo.logsFileLock.Unlock()

	f, err := os.OpenFile(LocalWigo.GetConfig().Global.EventLog, os.O_RDONLY, 0666)
	if err != nil {
		log.Printf("Critical : Failed to open events log file : %s", err)
		return
	}

	bf := bufio.NewReader(f)

	for {
		line, isPrefix, err := bf.ReadLine()

		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error reading file : %s\n", err)
		}
		if isPrefix {
			log.Printf("Error: Unexpected long line reading\n", f.Name())
		}

		newLog := new(Log)
		e := json.Unmarshal(line,newLog)
		if e != nil {
			break
		}

		if newLog.Timestamp != 0 {
			LocalWigo.AddLog(newLog, newLog.Level, newLog.Message)
		}
	}

	f.Close()
}


// Log levels
const (
	DEBUG		= 1
	NOTICE		= 2
	INFO		= 3
	ERROR		= 4
	WARNING		= 5
	CRITICAL 	= 6
	EMERGENCY 	= 7
)
