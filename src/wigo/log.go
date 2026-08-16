package wigo

import (
	"log"
	"os"
	"strings"
	"time"
)

var logFilehandle *os.File

type Log struct {
	Date      string
	Timestamp int64
	Level     uint8
	Message   string
	Host      string
	Probe     string
	Group     string
}

func NewLog(level uint8, message string) (this *Log) {
	this = new(Log)
	this.Date = time.Now().Format(dateLayout)
	this.Timestamp = time.Now().Unix()
	this.Level = level
	this.Message = message
	this.Host = GetLocalWigo().GetLocalHost().Name
	this.Group = GetLocalWigo().GetLocalHost().Group
	this.Probe = ""

	return
}

// Setters
func (this *Log) SetHost(hostname string) {
	this.Host = hostname
}
func (this *Log) SetGroup(group string) {
	this.Group = group
}

// Persist on disk
func (this *Log) Persist() {
	sqlStmt := `INSERT INTO logs(date,level,grp,host,probe,message) VALUES(?,?,?,?,?,?);`

	maxRetries := 3
	backoffDelays := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		LocalWigo.sqlLiteLock.Lock()
		_, err = LocalWigo.sqlLiteConn.Exec(sqlStmt, this.Timestamp, this.Level, this.Group, this.Host, this.Probe, this.Message)
		LocalWigo.sqlLiteLock.Unlock()

		if err == nil {
			return
		}

		// Check if error is SQLITE_BUSY or database locked
		errStr := err.Error()
		isBusyError := strings.Contains(errStr, "SQLITE_BUSY")

		if !isBusyError || attempt == maxRetries-1 {
			// Not a busy error or last attempt, log and return
			log.Printf("Fail to insert log in sqlLite : %s", err)
			return
		}

		// Wait before retry
		time.Sleep(backoffDelays[attempt])
	}
}

// Log levels
const (
	DEBUG     = 1
	NOTICE    = 2
	INFO      = 3
	ERROR     = 4
	WARNING   = 5
	CRITICAL  = 6
	EMERGENCY = 7
)
