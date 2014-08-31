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
    "wigo"
)

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

	// Create LocalHost
	localHost := wigo.NewLocalHost()

    // Log
    f, err := os.OpenFile( logFile, os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666 )
    if err != nil {
        fmt.Printf("Fail to open logfile %s : %s\n", logFile, err )
    } else {
        defer f.Close()

        log.SetOutput(f)
        log.SetPrefix(localHost.Name + " ")
    }

    // Signals
    signal.Notify( chanSignals, syscall.SIGINT, syscall.SIGTERM )

    // Result object
    globalResultsObject := make( map[string] *wigo.Host )
    globalResultsObject[ localHost.Name ] = localHost


    // Selection
    for{
        select {
            case e := <-chanWatch :
                chanChecks <- e

            case e := <-chanResults :
                if _, ok := e.Value.(*wigo.ProbeResult); ok {
                    probeResult := e.Value.(*wigo.ProbeResult)
                    globalResultsObject[ localHost.Name ].Probes[ probeResult.Name ] = probeResult
                    globalResultsObject[ localHost.Name ].GlobalStatus               = getGlobalStatus( globalResultsObject )
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
		return
    }

    // Create a watcher on checks directory
    err = watcher.Watch(checksDirectory)
    if err != nil {
        log.Fatal(err)
        return
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

					// Create local list of probes to detect removes
					currentProbesList,err := listProbesInDirectory(directory)
					if(err != nil){
						log.Printf("Fail to read directory %s : %s", directory, err )
					}

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

                            // Update probes list
                            newProbesList,err := listProbesInDirectory( directory )
                            if err != nil {
                                break
                            }

							// Check new probes
							for n := newProbesList.Front(); n != nil; n = n.Next() {
								newProbeName := n.Value.(string)
								probeIsNew := true

								// Add probe if new
								for j := currentProbesList.Front(); j != nil; j = j.Next() {
									probeName := j.Value.(string)

									if (probeName == newProbeName) {
										probeIsNew = false
									}
								}

								if(probeIsNew){
									currentProbesList.PushBack(newProbeName)
									log.Printf("Probe %s has been added in directory %s\n",newProbeName,directory)
								}
							}

							// Check deleted probes
							for c := currentProbesList.Front(); c != nil; c = c.Next() {
								probeName 			:= c.Value.(string)
								probeIsDeleted		:= true

								for n := newProbesList.Front(); n != nil; n = n.Next() {
									newProbeName 	:= n.Value.(string)

									if(probeName == newProbeName){
										probeIsDeleted = false
									}
								}

								if(probeIsDeleted){
									log.Printf("Probe %s has been deleted from filesystem.. Removing it from directory.\n", probeName )
									currentProbesList.Remove(c)
									continue
								}
							}

							// Launching probes
							log.Printf("Launching probes of directory %s",directory)

							for c := currentProbesList.Front(); c != nil; c = c.Next() {
								probeName := c.Value.(string)

								go execProbe( directory + "/" + probeName , probeResultsChannel, 5)
							}

                            // Sleep right amount of time
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

func listProbesInDirectory( directory string ) ( probesList *list.List, error error) {

	probesList = new(list.List)

    // List checks directory
    files, err := ioutil.ReadDir( directory )
    if err != nil {
        return nil, err
    }

    // Return only executables files
    for _, f := range files {
        if( ! f.IsDir() ){
            probesList.PushBack( f.Name() )
        }
    }

    return probesList, nil
}

func execProbe( probePath string, probeResultsChannel chan Event, timeOut int ){

    // Get probe name
    probeDirectory , probeName := path.Split( probePath )

    // Create ProbeResult
    var probeResult *wigo.ProbeResult

    // Create Command
    cmd := exec.Command( probePath )

    // Capture stdOut
    commandOutput := make([]byte,0)


    outputPipe, err := cmd.StdoutPipe()
    if err != nil {
        probeResult = wigo.NewProbeResult( probeName, 500, -1, fmt.Sprintf("error getting stdout pipe: %s",err), "")
        probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
        return
    }

    errPipe, err := cmd.StderrPipe()
    if err != nil {
        probeResult = wigo.NewProbeResult( probeName, 500, -1, fmt.Sprintf("error getting stderr pipe: %s",err), "")
        probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
        return
    }

    combinedOutput := io.MultiReader( outputPipe, errPipe )


    // Start
    err = cmd.Start()
    if err != nil {
        probeResult = wigo.NewProbeResult( probeName, 500, -1, fmt.Sprintf("error starting command: %s",err), "")
        probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
        return
    }

    // Wait channel
    done := make(chan error)
    go func(){
        commandOutput,err = ioutil.ReadAll(combinedOutput)
        if(err != nil){
            probeResult = wigo.NewProbeResult( probeName, 500, -1, fmt.Sprintf("error reading pipe: %s",err), "")
            probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
            return
        }

        done <-cmd.Wait()
    }()


    // Timeout or result ?
    select {
        case err := <-done :
            if(err != nil){
                probeResult = wigo.NewProbeResult( probeName, 500, -1, fmt.Sprintf("error: %s",err), string(commandOutput) )
                probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
                return

            } else {
                probeResult = wigo.NewProbeResultFromJson( probeName, commandOutput )
                probeResultsChannel <- Event{ NEWPROBERESULT , probeResult }
                log.Printf(" - Probe %s in directory %s responded with status : %d\n", probeResult.Name, probeDirectory, probeResult.Status)
                return
            }

        case <-time.After( time.Second * time.Duration(timeOut) ) :
            probeResult = wigo.NewProbeResult( probeName, 500, -1, "Probe timeout", "")
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

func getGlobalStatus( globalResultsObject map[string] *wigo.Host ) int {

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
    ADDDIRECTORY    	= 1
    REMOVEDIRECTORY 	= 2

    NEWCONNECTION   	= 5
    NEWPROBERESULT  	= 6
	DELETEPROBERESULT 	= 7

    SENDRESULTS     	= 10
)

type Event struct {
    Type int
    Value interface{}
}


