package main

import (
    "fmt"
    "log"
    "net"
    "os"
    "os/exec"
    "io/ioutil"
    "time"
    "syscall"
    "container/list"
    "encoding/json"

    "code.google.com/p/go.exp/inotify"
)

const dateLayout        = "Jan 2, 2006 at 3:04pm (MST)"
const listenAddr        = "localhost:4000"
const checksDirectory   = "checks"


func main() {

    // Channels
    chanWatch := make(chan Event)
    chanChecks := make(chan Event)
    chanSocket := make(chan Event)
    chanResults := make(chan Event)

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
    log.SetPrefix(localHostname + " ")


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

            case <-time.After( time.Second * 10 ) :
                log.Println("I'm main thread, and I'm still alive")
                Dump(globalResultsObject)
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
                log.Println("error:", err)
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
                    log.Println("Adding " , ev.Value)
                    checksDirectories.PushBack(ev.Value)

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

    // Launch tests
    for {

        for el := checksDirectories.Front(); el != nil ; el = el.Next(){
            currentDirectory := el.Value.(string)

            log.Println("Launching go routine check directory " , currentDirectory )

            // List probes in directory
            probesList,err := listProbesInDirectory( currentDirectory )
            if err != nil {
                break
            }

            // Iterate over directory
            for _,probe := range probesList {
                currentProbe := currentDirectory + "/" + probe

                // Exec probe
                out,err := exec.Command( currentProbe ).Output()
                if err != nil {
                    log.Println("   - Error executing probe " + currentProbe + " : " , err )
                }

                probeResult := NewProbeResultFromJson( out )

                // Tests
                if(probeResult.Name == ""){
                    continue
                }

                // Send result back to main thread
                log.Printf("   - Executed %s (%d : %s)\n", probeResult.Name, probeResult.Status, probeResult.Value )
                probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }

            }
        }

        time.Sleep( time.Second * 60 )
    }
}

func threadSocket( ci chan Event ) {

    // Listen
    listener, err := net.Listen("tcp", listenAddr)
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
    ci <- Event{ NEWCONNECTION , "thread-socket-newConnection" }

    // Wait
    ev := <-ci
    if(ev.Type == SENDRESULTS){
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
func stringInSlice(a string, list []string) bool {
    for _, b := range list {
        if b == a {
            return true
        }
    }
    return false
}
func Dump( data interface{}){
    json,_ := json.MarshalIndent(data,"","")
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
    Status      int
}

func NewProbeResultFromJson( ba []byte ) ( this *ProbeResult ){
    this = new( ProbeResult )

    // Fill from json
    json.Unmarshal( ba, this )

    // Add date
    this.ProbeDate = time.Now().Format(dateLayout)


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
