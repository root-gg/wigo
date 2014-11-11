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

	"github.com/nu7hatch/gouuid"
)


type PushServer struct {
	config		*PushServerConfig
	server		*rpc.Server
	cartman 	*Cartman
	tokens  	map[string]string
}

func NewPushServer( config *PushServerConfig ) ( this *PushServer ) {
	this = new(PushServer)

	this.tokens = make(map[string]string)

	this.config = config
	address := this.config.Address+":"+strconv.Itoa(config.Port)
	this.cartman = NewCartman(this.config)

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
			log.Fatal("Push server error : ", err)
		}
		rawListner, err := net.Listen("tcp", address)
		if err != nil {
			log.Fatal("Push server error : ", err)
		}
		listner = tls.NewListener(rawListner, tlsConfig);

		log.Println("Tls push server now listening @ " + address)
	} else {
		listner, err = net.Listen("tcp", this.config.Address+":"+strconv.Itoa(this.config.Port)) ;
		if err != nil {
			log.Fatal("Push server error : ", err)
		}
		log.Println("Plain push server now listening @ " + address)
	}

	go func() {
		for {
			if conn, err := listner.Accept() ; err == nil {
	 			go rpc.ServeConn(conn)
			} else {
				log.Println(err)
			}
		}
	}()
	return
}

func (this *PushServer) Test(str string, str2 *string) (err error) {
	log.Println("revc : " + str)
	*str2 = str
	return
}

func (this *PushServer) Yo(req HelloRequest, reply *bool) ( err error ) {
	Dump(req)
	if this.cartman.IsAllowed(req.Uuid) {
		log.Println("Push server : " + req.Hostname + " is already allowed")
	} else {
		this.cartman.AddToWaitingList(req.Uuid,req.Hostname)
		if ( this.config.AutoAcceptClients ) {
			this.cartman.Allow(req.Uuid, req.Hostname)
		}
	}
	return
}

func (this *PushServer) GetServerCertificate(req HelloRequest, cert *[]byte) ( err error ) {
	Dump(req)
	log.Println("Push server : Sending server certificate to " + req.Hostname);
	*cert = this.cartman.GetServerCertificate()
	return
}

func (this *PushServer) Autograph(req HelloRequest, autograph *[]byte) ( err error ) {
	Dump(req)
	if this.cartman.IsAllowed(req.Uuid) {
		log.Println("Push server : Sending autograph to " + req.Hostname);
		*autograph, err = this.cartman.GetAutograph(req.Uuid, req.Hostname)
	} else {
		err = errors.New("NOT ALLOWED")
	}
	return
}

type HelloRequest struct {
	Hostname 	string
	Uuid		string
	Autograph	[]byte
}

func NewHelloRequest(autograph []byte) ( this *HelloRequest ){
	this = new(HelloRequest)
	this.Hostname = LocalWigo.GetHostname()
	this.Uuid = LocalWigo.Uuid
	this.Autograph = autograph
	return
}

func (this *PushServer) Hello(req HelloRequest, token *string) ( err error ) {
	Dump(req)
	if this.cartman.IsAllowed(req.Uuid) {
		if err = this.cartman.VerifyAutograph(req.Uuid, req.Autograph) ; err == nil {
			if t, err := uuid.NewV4() ; err == nil {
				*token = t.String()
				this.tokens[*token] = req.Uuid
				log.Println("Push server : hello " + req.Hostname)
			} else {
				log.Println("Unable to generate push token : " + err.Error())
				err = errors.New("NOT ALLOWED")
			}
		} else {
			log.Println("Push server : invalid autograph " + req.Hostname)
			err = errors.New("NOT ALLOWED")
		}
	} else {
		err = errors.New("NOT ALLOWED")
	}

	return
}

/*
 * Base request
 */
type Request struct {
	Token 	   	string
	wigo		*Wigo
}

func NewRequest(token string) ( this *Request ) {
	this = new(Request)
	this.Token = token
	return
}

func (this *PushServer) auth(req *Request) ( err error ) {
	Dump(req)
	if uuid, ok := this.tokens[req.Token] ; ok {
		if req.wigo = LocalWigo.FindRemoteWigoByUuid(uuid) ; req.wigo != nil {
			// Check session validity
			// TODO implement anti flood
			if time.Now().Unix() - req.wigo.LastUpdate > int64(300) {
				log.Println("Push server : session timed out for " + req.wigo.GetHostname())
				err = errors.New("NOT ALLOWED")
			}
		}
	} else {
		log.Println("Push server : invalid token " + req.Token)
		err = errors.New("NOT ALLOWED")
	}

	return
}

/*
 * Update request
 */
type UpdateRequest struct {
	*Request
	Wigo		Wigo
}

func NewUpdateRequest(wigo *Wigo, token string) (this *UpdateRequest) {
	this = new(UpdateRequest)
	this.Request = NewRequest(token)
	this.Wigo = *wigo
	return
}

func (this *PushServer) Update(req UpdateRequest, reply *bool) (err error) {
	Dump(req)
	if err := this.auth(req.Request) ; err == nil {
		LocalWigo.AddOrUpdateRemoteWigo(req.Wigo.GetHostname(), &req.Wigo)
	}
	return
}

func (this *PushServer) Goodbye(req Request, reply *bool) ( err error ) {
	Dump(req)
	if err := this.auth(&req) ; err == nil {
		delete(this.tokens,req.Token)
		req.wigo.IsAlive = false;
	}
	return
}
