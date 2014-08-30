package main

import (
    "fmt"
    "log"
    "net"
    "os"
    "os/exec"
    "os/signal"
    "io"
    "io/ioutil"
    "time"
    "path"
    "syscall"
    "strconv"
    "container/list"
    "encoding/json"

    "code.google.com/p/go.exp/inotify"
)

const dateLayout        = "Jan 2, 2006 at 3:04pm (MST)"
const listenProto       = "tcp4"
const listenAddr        = ":4000"
const checksDirectory   = "/usr/local/wigo/probes"
const logFile           = "/var/log/wigo.log"

func main() {

    // Channels
    chanWatch := make(chan Event)
    chanChecks := make(chan Event)
    chanSocket := make(chan Event)
    chanResults := make(chan Event)
    chanSignals := make(chan os.Signal)

    // Launch goroutines
    go threadWatch(chanWatch)
    go threadLocalChecks(chanChecks,chanResults)
    go threadSocket(chanSocket)


    // Get hostname
    localHostname, err := os.Hostname()
    if( err != nil ){
        log.Fatal("Can't get hostname from local machine : " , err )
        os.Exit(1)
    }


    // Log
    f, err := os.OpenFile( logFile, os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666 )
    if err != nil {
        fmt.Printf("Fail to open logfile %s : %s", logFile, err )
    } else {
        defer f.Close()

        log.SetOutput(f)
        log.SetPrefix(localHostname + " ")
    }


    // Signals
    signal.Notify( chanSignals, syscall.SIGINT, syscall.SIGTERM )


    // Result object
    globalResultsObject := make( map[string] *Host )
    globalResultsObject[ localHostname ] = NewHost( localHostname )


    // Selection
    for{
        select {
            case e := <-chanWatch :
                chanChecks <- e

            case e := <-chanResults :
                if _, ok := e.Value.(*ProbeResult); ok {
                    probeResult := e.Value.(*ProbeResult)
                    globalResultsObject[ localHostname ].Probes[ probeResult.Name ] = probeResult
                    globalResultsObject[ localHostname ].GlobalStatus               = getGlobalStatus( globalResultsObject )
                }

            case e := <-chanSocket :
                switch e.Type {
                    case NEWCONNECTION :
                        log.Printf("Got a new connection : %s\n" , e.Value)

                        // Send json to socket channel
                        j, err := json.MarshalIndent(globalResultsObject,"","    ")
                        if( err != nil ){
                            log.Println("Fail to encode to json : " , err )
                            break
                        }

                        chanSocket <- Event{ SENDRESULTS , j }
                }

            case <-chanSignals :
                os.Exit(0)
        }
    }
}


//
//// Threads
//

func threadWatch( ci chan Event ) {
    // Vars
    checkDirectories := make( []string, 0 )

    // First list
    checkDirectories, err := listChecksDirectories()

    // Send
    for _, dir := range checkDirectories {
        ci <- Event{ ADDDIRECTORY, checksDirectory + "/" + dir }
    }

    // Init inotify
    watcher, err := inotify.NewWatcher()
    if err != nil {
        log.Fatal(err)
        os.Exit(1)
    }

    // Create a watcher on checks directory
    err = watcher.Watch(checksDirectory)
    if err != nil {
        log.Fatal(err)
        os.Exit(1)
    }

    // Watch for changes forever
    for {
        select {
            case ev := <-watcher.Event:
                switch ev.Mask {
                    case syscall.IN_CREATE|syscall.IN_ISDIR :
                        ci <- Event{ADDDIRECTORY, ev.Name}
                    case syscall.IN_DELETE|syscall.IN_ISDIR :
                        ci <- Event{REMOVEDIRECTORY, ev.Name}
                }
            case err := <-watcher.Error:
                log.Println("directoryWatcher:", err)
        }
    }
}

func threadLocalChecks( ci chan Event , probeResultsChannel chan Event ) {

    // Directory list
    var checksDirectories list.List


    // Listen events
    go func(){
        for {
            ev := <-ci

            switch ev.Type {
                case ADDDIRECTORY :

                    var directory string = ev.Value.(string)

                    log.Println("Adding directory" , directory)
                    checksDirectories.PushBack(directory)

                    go func(){
                        for{
                            // Am I still a valid directory ?
                            stillValid := false
                            for e := checksDirectories.Front(); e != nil; e = e.Next() {
                                if(e.Value == directory){
                                    stillValid = true
                                }
                            }
                            if(!stillValid){
                                return
                            }


                            // Guess sleep time from dir
                            sleepTime           := path.Base( directory )
                            sleepTImeInt, err   := strconv.Atoi( sleepTime )
                            if(err != nil){
                                log.Printf(" - Weird folder name %s. Doing nothing...\n", directory)
                                return
                            }

                            // List probes in directory
                            probesList,err := listProbesInDirectory( directory )
                            if err != nil {
                                break
                            }

                            // Iterate over directory
                            for _,probe := range probesList {
                                go execProbe( directory + "/" + probe , probeResultsChannel, 5)
                            }

                            // Sleep right amount of time
                            log.Printf(" - Launched checks from directory %s. Sleeping %d seconds...\n", directory, sleepTImeInt)
                            time.Sleep( time.Second * time.Duration(sleepTImeInt) )

                        }
                    }()

                case REMOVEDIRECTORY :
                    for el := checksDirectories.Front(); el != nil ; el = el.Next(){
                        if(el.Value == ev.Value){
                            log.Println("Removing " , ev.Value)
                            checksDirectories.Remove(el)
                            break
                        }
                    }
            }
        }
    }()
}

func threadSocket( ci chan Event ) {

    // Listen
    listener, err := net.Listen(listenProto, listenAddr)
    if err != nil {
        log.Fatal(err)
        os.Exit(1)
    }

    // Serve
    for {
        c, err := listener.Accept()
        if err != nil {
            log.Fatal(err)
        }

        go handleRequest(c,ci)
    }
}




func handleRequest( c net.Conn, ci chan Event ) error {

    // Request json
    ci <- Event{ NEWCONNECTION , c.RemoteAddr() }

    // Wait
    ev := <-ci
    if(ev.Type == SENDRESULTS){

        // Send results
        fmt.Fprintln(c , string(ev.Value.([]byte)))
    }

    return c.Close()
}


func listChecksDirectories() ([]string,error) {

    // List checks directory
    files, err := ioutil.ReadDir(checksDirectory)
    if err != nil {
        return nil, err
    }

    // Init array
    subdirectories := make([]string, 0)


    // Return only subdirectories
    for _, f := range files {
        if(f.IsDir()){
            subdirectories = append( subdirectories, f.Name())
        }
    }

    return subdirectories, nil
}

func listProbesInDirectory( directory string) ([]string,error) {

    // List checks directory
    files, err := ioutil.ReadDir( directory )
    if err != nil {
        return nil, err
    }

    // Init array
    probesList := make([]string, 0)


    // Return only executables files
    for _, f := range files {
        if( ! f.IsDir() ){
            probesList = append( probesList, f.Name())
        }
    }

    return probesList, nil
}

func execProbe( probePath string, probeResultsChannel chan Event, timeOut int ){

    // Get probe name
    probeDirectory , probeName := path.Split( probePath )

    // Create ProbeResult
    var probeResult *ProbeResult

    // Create Command
    cmd := exec.Command( probePath )

    // Capture stdOut
    commandOutput := make([]byte,0)

    outputPipe, err := cmd.StdoutPipe()
    if err != nil {
        probeResult = NewProbeResult( probeName, 500, -1, fmt.Sprintf("error getting stdout pipe: %s",err), "")
        probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
        return
    }

    errPipe, err := cmd.StderrPipe()
    if err != nil {
        probeResult = NewProbeResult( probeName, 500, -1, fmt.Sprintf("error getting stderr pipe: %s",err), "")
        probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
        return
    }

    combinedOutput := io.MultiReader( outputPipe, errPipe )


    // Start
    err = cmd.Start()
    if err != nil {
        probeResult = NewProbeResult( probeName, 500, -1, fmt.Sprintf("error starting command: %s",err), "")
        probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
        return
    }

    // Wait channel
    done := make(chan error)
    go func(){
        commandOutput,err = ioutil.ReadAll(combinedOutput)
        if(err != nil){
            probeResult = NewProbeResult( probeName, 500, -1, fmt.Sprintf("error reading pipe: %s",err), "")
            probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
            return
        }

        done <-cmd.Wait()
    }()


    // Timeout or result ?
    select {
        case err := <-done :
            if(err != nil){
                probeResult = NewProbeResult( probeName, 500, -1, fmt.Sprintf("error: %s",err), string(commandOutput) )
                probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
                return

            } else {
                probeResult = NewProbeResultFromJson( probeName, commandOutput )
                probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
                log.Printf(" - Probe %s in directory %s responded with status : %d\n", probeResult.Name, probeDirectory, probeResult.Status)
                return
            }

        case <-time.After( time.Second * time.Duration(timeOut) ) :
            probeResult = NewProbeResult( probeName, 500, -1, "Probe timeout", "")
            probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
            log.Printf(" - Probe %s in directory %s timeouted..\n", probeResult.Name, probeDirectory )

            // Killing it..
            log.Printf(" - Killing it...")
            err := cmd.Process.Kill()
            if(err != nil){
                log.Printf(" - Failed to kill probe %s : %s\n", probeName, err)
                // TODO handle error
            } else {
                log.Printf(" - Probe %s successfully killed\n", probeName)
            }

            return
    }

}

func getGlobalStatus( globalResultsObject map[string] *Host ) int {

    globalStatus := 100

    for hostname := range globalResultsObject {
        host    := globalResultsObject[ hostname ]

        for _, probe := range host.Probes {
            if(probe.Status > globalStatus){
                globalStatus = probe.Status
            }
        }
    }

    return globalStatus
}


// Misc
func Dump( data interface{}){
    json,_ := json.MarshalIndent(data,"","   ")
    fmt.Printf("%s\n",string(json))
}


//
//// STRUCTURES
//

// Events

const (
    ADDDIRECTORY    = 1
    REMOVEDIRECTORY = 2

    NEWCONNECTION   = 5
    NEWPROBERESULT  = 6

    SENDRESULTS     = 10
)

type Event struct {
    Type int
    Value interface{}
}


// Probe  Results

type ProbeResult struct {

    Name        string
    Version     string
    Value       interface{}
    Message     string
    Detail      string
    ProbeDate   string
    Metrics     map[string]float64

    Status      int
    ExitCode    int
}

func NewProbeResultFromJson( name string, ba []byte ) ( this *ProbeResult ){
    this = new( ProbeResult )

    json.Unmarshal( ba, this )

    this.Name      = name
    this.ProbeDate = time.Now().Format(dateLayout)
    this.ExitCode  = 0

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

    return
}


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
