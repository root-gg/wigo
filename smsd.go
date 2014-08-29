package main

import (
    "fmt"
    "log"
    "net"
    "os"
    "os/exec"
    "io/ioutil"
    "time"
)

const listenAddr        = "localhost:4000"
const checksDirectory   = "checks"


func main() {

    // Channels
    chanWatch := make(chan []string)
    chanChecks := make(chan string)
    chanSocket := make(chan string)

    // Launch goroutines
    go threadWatch(chanWatch)
    go threadChecks(chanChecks)
    go threadSocket(chanSocket)

    // Selection
    for{
        select {
            case msg := <-chanWatch :
                fmt.Printf("[MAIN  ] Changes detected on checks directory %s\n", msg )

            case msg := <-chanChecks :
                fmt.Printf("[MAIN  ] Received something from cheks channel : %s\n" , msg)

            case msg := <-chanSocket :
                fmt.Printf("[MAIN  ] Received something from socket channel : %s\n" , msg)
                fmt.Printf("[MAIN  ] -> Sending json\n")
                chanSocket <- "lol"
        }
    }
}


//
//// Threads
//

func threadWatch( ci chan []string ) {

    // Vars
    checkDirectories := make( []string, 0 )

    for {

        isChanged := false

        // Update directories
        checkDirectoriesReloaded, err := listChecksDirectories()
        if err != nil {
            log.Fatal(err)
            os.Exit(1)
        }

        // Delete old ones
        deletedDirectories := make( []string, 0 )
        for _,dir := range checkDirectories {
            if( ! stringInSlice(dir,checkDirectoriesReloaded) ){
                isChanged = true
                fmt.Printf("[WATCH ] Deleting directory %s\n" , dir )
                deletedDirectories = append(deletedDirectories, dir)
            }
        }

        // Check for new ones
        newCheckDirectories := make( []string, 0 )
        for _,dir := range checkDirectoriesReloaded {
            newCheckDirectories = append(newCheckDirectories, dir)

            if( ! stringInSlice(dir,checkDirectories) ){
                isChanged = true
                fmt.Printf("[WATCH ] Adding directory %s\n" , dir )
            }
        }

        // Set new list
        checkDirectories = newCheckDirectories

        // If changed, tell main
        if(isChanged){
            ci <- checkDirectories
        }

        time.Sleep( time.Second )
    }
}
func threadChecks( ci chan string ) {
    for {
        ci <- "thread-checks-keepalive"
        time.Sleep( time.Minute )
    }
}
func threadSocket( ci chan string ) {

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

        ci <- "thread-socket-newConnection"
        msg := <-ci

        fmt.Printf("[SOCKET] Received msg from main thread : %s \n",msg)

        go handleRequest(c)
    }
}




func handleRequest( c net.Conn ) error {
    checkOutput, err := runCheck("/home/mbodjiki/hardware_load_avg")

    myHostname, err := os.Hostname()

    if err != nil {
        fmt.Fprintf(c,"Error : %s",err  )
    } else {
        fmt.Fprintln(c,myHostname)
        fmt.Fprintln(c,checkOutput)

        checkDirectories, err := listChecksDirectories()
        if err != nil {
        }

        for _ , directory := range checkDirectories {
            fmt.Fprintln(c, "Got a directory " + directory )
        }

    }
    return c.Close()
}


func runCheck( path string ) (string,error) {
    out, err := exec.Command( path ).Output()
    return string(out),err
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



// Misc
func stringInSlice(a string, list []string) bool {
    for _, b := range list {
        if b == a {
            return true
        }
    }
    return false
}
