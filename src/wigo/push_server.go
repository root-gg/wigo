package wigo

import (
	"net"
	"log"
	"net/rpc"
	"strconv"
	"errors"
	"encoding/gob"
	"crypto/tls"
	"time"
)

// Push server expose method to update client's
// data over RPCs. Data is transferred using binary
// gob serialisation over tcp connection. Secure TLS
// connection is available and highly recommended.
type PushServer struct {
	config		*PushServerConfig
	server		*rpc.Server
	authority 	*Authority
}

func NewPushServer(config *PushServerConfig ) ( this *PushServer ) {
	this = new(PushServer)

	this.config = config
	address := this.config.Address + ":" + strconv.Itoa(config.Port)
	this.authority = NewAuthority(this.config)

	gob.Register([]interface {}{})
	gob.Register(map[string]interface {}{})
	rpc.Register(this)

	var listner net.Listener
	var err error
	if (this.config.SslEnabled) {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(this.config.SslCert,this.config.SslKey)
		if err != nil {
			log.Fatalf("Push server : error while loading server certificate : %s", err)
		}
		rawListner, err := net.Listen("tcp", address)
		if err != nil {
			log.Fatalf("Push server : listen error : %s", err)
		}
		listner = tls.NewListener(rawListner, tlsConfig);

		log.Printf("Push server : now listening @ %s ( TLS enabled )", address)
	} else {
		listner, err = net.Listen("tcp", this.config.Address+":"+strconv.Itoa(this.config.Port)) ;
		if err != nil {
			log.Fatalf("Push server : listen error %s", err)
		}
		log.Println("Push server : now listening @ %s ( TLS disabled ! )", address)
	}

	go func() {
		for {
			if conn, err := listner.Accept() ; err == nil {
	 			go rpc.ServeConn(conn)
			} else {
				log.Printf("Push server : accept connection error %s", err)
			}
		}
	}()
	return
}

// PUSH SERVER RPCs

// Send the server CA certificate to the client so it can
// verify the identity of the server. To avoid the small window
// of MITM vulnerability you might copy the certificate by yourself.
func (this *PushServer) GetServerCertificate(req HelloRequest, cert *[]byte) ( err error ) {
	Dump(req)
	log.Println("Push server : Sending server certificate to " + req.Hostname);
	*cert = this.authority.GetServerCertificate()
	return
}

// Register a new client. It will first be added to a 
// waiting list, then an admin action will be required
// to grant the client to the allowed list. You may accept
// new clients automatically with the AutoAcceptClient setting.
func (this *PushServer) Register(req HelloRequest, reply *bool) ( err error ) {
	Dump(req)
	this.authority.AddClientToWaitingList(req.Uuid,req.Hostname)
	if ( this.config.AutoAcceptClients ) {
		this.authority.AllowClient(req.Uuid)
	}
	return
}

// Sign the client uuid with the server's private key.
// The client will have to provide this as a proof of
// his identity at every new connection.
func (this *PushServer) GetUuidSignature(req HelloRequest, sig *[]byte) ( err error ) {
	Dump(req)
	if this.authority.IsAllowed(req.Uuid) {
		log.Println("Push server : Sending uuid signature to " + req.Hostname);
		*sig, err = this.authority.GetUuidSignature(req.Uuid, req.Hostname)
	} else {
		if this.authority.IsWaiting(req.Uuid) {
			err = errors.New("WAITING")
		} else {
			err = errors.New("NOT ALLOWED")
		}
	}
	return
}

// Verify the validity of the client's uuid signature. This is done 
// once for every connection then a token then a token is used.
func (this *PushServer) Hello(req HelloRequest, token *string) ( err error ) {
	Dump(req)
	if this.authority.IsAllowed(req.Uuid) {
		if err = this.authority.VerifyUuidSignature(req.Uuid, req.UuidSignature) ; err == nil {
			if *token, err = this.authority.GetToken(req.Uuid) ; err == nil{
				log.Printf("Push server : hello %s", req.Hostname)
			} else {
				err = errors.New("NOT ALLOWED")
			}
		} else {
			log.Println("Push server : invalid uuid signature for " + req.Hostname)
			err = errors.New("NOT ALLOWED")
		}
	} else {
		if this.authority.IsWaiting(req.Uuid) {
			err = errors.New("WAITING")
		} else {
			err = errors.New("NOT ALLOWED")
		}
	}

	return
}

// Update a client's data
func (this *PushServer) Update(req UpdateRequest, reply *bool) (err error) {
	Dump(req)
	if err = this.auth(req.Request) ; err == nil {
		// TODO this should return an error
		LocalWigo.AddOrUpdateRemoteWigo(req.Wigo.GetHostname(), &req.Wigo)
	} else {
		err = errors.New("NOT ALLOWED")
	}
	return
}

// Disconnect the client gracefully
func (this *PushServer) Goodbye(req Request, reply *bool) ( err error ) {
	Dump(req)
	if err = this.auth(&req) ; err == nil {
		this.authority.RevokeToken(req.Uuid,req.Token)
		if wigo := LocalWigo.FindRemoteWigoByUuid(req.Uuid) ; wigo != nil {
			wigo.IsAlive = false
		}
	}
	return
}

// As the signature of RPC methods is not flexible
// Input parameter are encapsulated into requests
// objects.

// Hello request for the first request
type HelloRequest struct {
	Hostname 		string
	Uuid			string
	UuidSignature	[]byte
}

func NewHelloRequest(uuidSignature []byte) ( this *HelloRequest ){
	this = new(HelloRequest)
	this.Hostname = LocalWigo.GetHostname()
	this.Uuid = LocalWigo.Uuid
	this.UuidSignature = uuidSignature
	return
}

// Base request for every subsequent requests
type Request struct {
	Uuid		string
	Token 	   	string
}

func NewRequest(uuid string, token string) ( this *Request ) {
	this = new(Request)
	this.Uuid = uuid
	this.Token = token
	return
}

// This check the validity of the token. Token will
// expire within 300 seconds hence forcing the client
// to reconnect. Here we also check for flooding clients.
func (this *PushServer) auth(req *Request) ( err error ) {
	Dump(req)
	err = this.authority.VerifyToken(req.Uuid,req.Token)
	if err == nil {
		if wigo := LocalWigo.FindRemoteWigoByUuid(req.Uuid) ; wigo != nil {
			// TODO implement anti flood
			if time.Now().Unix() - wigo.LastUpdate > int64(300) {
				log.Printf("Push server : session timed out for %s", wigo.GetHostname())
				err = errors.New("NOT ALLOWED")
			}
		}
	} else {
		err = errors.New("NOT ALLOWED")
	}

	return
}

// Request the server to update the client's data
type UpdateRequest struct {
	*Request
	Wigo		Wigo
}

func NewUpdateRequest(wigo *Wigo, token string) (this *UpdateRequest) {
	this = new(UpdateRequest)
	this.Request = NewRequest(wigo.Uuid, token)
	this.Wigo = *wigo
	return
}
