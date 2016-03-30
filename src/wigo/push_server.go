package wigo

import (
	"crypto/tls"
	"encoding/gob"
	"errors"
	"log"
	"net"
	"net/rpc"
	"strconv"
)

// Push server expose method to update client's
// data over RPCs. Data is transferred using binary
// gob serialisation over tcp connection. Secure TLS
// connection is available and highly recommended.
type PushServer struct {
	config    *PushServerConfig
	server    *rpc.Server
	authority *Authority
}

func NewPushServer(config *PushServerConfig) (this *PushServer) {
	this = new(PushServer)

	this.config = config
	address := this.config.Address + ":" + strconv.Itoa(config.Port)
	this.authority = NewAuthority(this.config)

	gob.Register([]interface{}{})
	gob.Register(map[string]interface{}{})
	rpc.Register(this)

	var listener net.Listener
	var err error
	if this.config.SslEnabled {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(this.config.SslCert, this.config.SslKey)
		if err != nil {
			log.Fatalf("Push server : error while loading server certificate from %s : %s", this.config.SslCert, err)
		}
		rawListner, err := net.Listen("tcp", address)
		if err != nil {
			log.Fatalf("Push server : listen error : %s", err)
		}
		listener = tls.NewListener(rawListner, tlsConfig)

		log.Printf("Push server : now listening @ %s ( TLS enabled )", address)
	} else {
		listener, err = net.Listen("tcp", this.config.Address+":"+strconv.Itoa(this.config.Port))
		if err != nil {
			log.Fatalf("Push server : listen error %s", err)
		}
		log.Printf("Push server : now listening @ %s ( TLS disabled ! )", address)
	}

	go func() {
		for {
			if conn, err := listener.Accept(); err == nil {
				log.Printf("Push server [client %s] : accepting connection", conn.RemoteAddr())
				go func() {
					rpc.ServeConn(conn);
					log.Printf("Push server [client %s] : closing connection", conn.RemoteAddr())
					conn.Close()
				}()
			} else {
				log.Printf("Push server [client %s] : accept connection failed : %s", conn.RemoteAddr(), err)
			}
		}
	}()
	return
}

// PUSH SERVER RPCs

// Send the server CA certificate to the client so it can
// verify the identity of the server. To avoid the small window
// of MITM vulnerability you might copy the certificate by yourself.
func (this *PushServer) GetServerCertificate(req HelloRequest, cert *[]byte) (err error) {
	//	if LocalWigo.GetConfig().Global.Debug {
	//		Dump(req)
	//	}
	log.Printf("Push server [client %s] : sending server certificate", req.Hostname)
	*cert = this.authority.GetServerCertificate()
	return
}

// Register a new client. It will first be added to a
// waiting list, then an admin action will be required
// to grant the client to the allowed list. You may accept
// new clients automatically with the AutoAcceptClient setting.
func (this *PushServer) Register(req HelloRequest, reply *bool) (err error) {
	//	if LocalWigo.GetConfig().Global.Debug {
	//		Dump(req)
	//	}
	if !this.authority.IsAllowed(req.Uuid) {
		log.Printf("Push server [client %s] : adding client to waiting list", req.Hostname)
		this.authority.AddClientToWaitingList(req.Uuid, req.Hostname)
		if this.config.AutoAcceptClients {
			log.Printf("Push server [client %s] : automatically accepting client as configured", req.Hostname)
			this.authority.AllowClient(req.Uuid)
		}
	}
	return
}

// Sign the client uuid with the server's private key.
// The client will have to provide this as a proof of
// his identity at every new connection.
func (this *PushServer) GetUuidSignature(req HelloRequest, sig *[]byte) (err error) {
	//	if LocalWigo.GetConfig().Global.Debug {
	//		Dump(req)
	//	}
	if this.authority.IsAllowed(req.Uuid) {
		log.Printf("Push server [client %s] : sending uuid signature", req.Hostname)
		*sig, err = this.authority.GetUuidSignature(req.Uuid, req.Hostname)
	} else {
		if this.authority.IsWaiting(req.Uuid) {
			log.Printf("Push server [client %s] : won't sign your uuid, you're on the waiting queue", req.Hostname)
			err = errors.New("WAITING")
		} else {
			log.Printf("Push server [client %s] : won't sign your uuid, you're not allowed", req.Hostname)
			err = errors.New("NOT ALLOWED")
		}
	}
	return
}

// Verify the validity of the client's uuid signature. This is done
// once for every connection then a token then a token is used.
func (this *PushServer) Hello(req HelloRequest, token *string) (err error) {
	//	if LocalWigo.GetConfig().Global.Debug {
	//		Dump(req)
	//	}
	if this.authority.IsAllowed(req.Uuid) {
		if err = this.authority.VerifyUuidSignature(req.Uuid, req.UuidSignature); err == nil {
			if *token, err = this.authority.GetToken(req.Uuid); err == nil {
				log.Printf("Push server [client %s] : Hello", req.Hostname)
			} else {
				log.Printf("Push server [client %s] : Hello, your uuid is valid but couldn't get your token (%s)", req.Hostname, err.Error())
				err = errors.New("NOT ALLOWED")
			}
		} else {
			log.Printf("Push server [client %s] : Hello, your uuid signature is invalid (%s)", req.Hostname, err.Error())
			err = errors.New("NOT ALLOWED")
		}
	} else {
		if this.authority.IsWaiting(req.Uuid) {
			log.Printf("Push server [client %s] : Hello, you're in the waiting queue", req.Hostname)
			err = errors.New("WAITING")
		} else {
			log.Printf("Push server [client %s] : Hello, you're not allowed", req.Hostname)
			err = errors.New("NOT ALLOWED")
		}
	}

	return
}

// Update a client's data
func (this *PushServer) Update(req UpdateRequest, reply *bool) (err error) {
	//	if LocalWigo.GetConfig().Global.Debug {
	//		Dump(req)
	//	}
	if err = this.auth(req.Request); err == nil {
		log.Printf("Push server : Update from %s", req.Wigo.GetHostname())
		req.Wigo.SetParentHostsInProbes()
		// TODO this should return an error
		LocalWigo.AddOrUpdateRemoteWigo(&req.Wigo)
	} else {
		log.Printf("Push server : Update for %s refused, you're not allowed", req.Wigo.GetHostname())
		err = errors.New("NOT ALLOWED")
	}
	return
}

// Disconnect the client gracefully
func (this *PushServer) Goodbye(req Request, reply *bool) (err error) {
	//	if LocalWigo.GetConfig().Global.Debug {
	//		Dump(req)
	//	}
	if err = this.auth(&req); err == nil {
		this.authority.RevokeToken(req.Uuid, req.Token)
	}
	return
}

// As the signature of RPC methods is not flexible
// Input parameter are encapsulated into requests
// objects.

// Hello request for the first request
type HelloRequest struct {
	Hostname      string
	Uuid          string
	UuidSignature []byte
}

func NewHelloRequest(uuidSignature []byte) (this *HelloRequest) {
	this = new(HelloRequest)
	this.Hostname = LocalWigo.GetHostname()
	this.Uuid = LocalWigo.Uuid
	this.UuidSignature = uuidSignature
	return
}

// Base request for every subsequent requests
type Request struct {
	Uuid  string
	Token string
}

func NewRequest(uuid string, token string) (this *Request) {
	this = new(Request)
	this.Uuid = uuid
	this.Token = token
	return
}

// This check the validity of the token. Token will
// expire within 300 seconds hence forcing the client
// to reconnect. Here we also check for flooding clients.
func (this *PushServer) auth(req *Request) (err error) {
	//	if LocalWigo.GetConfig().Global.Debug {
	//		Dump(req)
	//	}
	err = this.authority.VerifyToken(req.Uuid, req.Token)
	if err != nil {
		err = errors.New("NOT ALLOWED")
	}

	return
}

// Request the server to update the client's data
type UpdateRequest struct {
	*Request
	Wigo Wigo
}

func NewUpdateRequest(wigo *Wigo, token string) (this *UpdateRequest) {
	this = new(UpdateRequest)
	this.Request = NewRequest(wigo.Uuid, token)
	this.Wigo = *wigo
	return
}
