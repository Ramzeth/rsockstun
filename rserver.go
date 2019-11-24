package main

import (
	"crypto/tls"
	"fmt"
	"golang.org/x/net/websocket"
	"io"
	"log"
	"net"
	"os"

	"bufio"
	"time"
	//"encoding/hex"
	"github.com/hashicorp/yamux"
	"net/http"
	"strings"
)
//var session *yamux.Session
var stream *yamux.Stream
var proxytout = time.Millisecond * 2000 //timeout for wait for password
var rurl string//redirect URL

func copyWorker(dst io.Writer, src io.Reader, doneCh chan<- bool) {
	io.Copy(dst, src)
	doneCh <- true
}

func wsHandler(ws *websocket.Conn) {
	log.Printf("Got ws connection from  %v \n", ws.RemoteAddr())
	var err error

	ws.PayloadType = websocket.BinaryFrame
	reader := bufio.NewReader(ws)

	//read only 64 bytes with timeout=1-3 sec. So we haven't delay with browsers
	ws.SetReadDeadline(time.Now().Add(proxytout))
	statusb := make([]byte,64)
	_,_ = io.ReadFull(reader,statusb)

	if string(statusb)[:len(agentpassword)] != agentpassword {
		//if passwordis not correct
		log.Println("Password is incorrect.")
		ws.Close()
	}else {
		//password is correct
		//disable socket read timeouts
		log.Printf("Auth remote ws client from %v. \n", ws.RemoteAddr())
		err = ws.SetReadDeadline(time.Now().Add(100 * time.Hour))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error...")
		}

		//connect with yamux
		//Add connection to yamux
		yconf := yamux.DefaultConfig()
		//yconf.EnableKeepAlive = false
		yconf.KeepAliveInterval =  time.Millisecond * 50000
		session, err = yamux.Client(ws, yconf)
		for {
			time.Sleep(time.Second * 1)
			if session.IsClosed() {
				//log.Println("Debug: End handler.")
				return
			}
		}
	}
}

func listenForWsClients(address string, certFile string){
	address = strings.Replace(address,"ws:","",1)
	address = strings.Replace(address,"wss:","",1)
	//create and set http handler
	http.Handle("/", websocket.Handler(wsHandler))
	var err error
	if certFile != ""  {
		log.Println("Listening for https websocket far end...")
		err = http.ListenAndServeTLS(address, certFile+".crt", certFile+".key", nil)
	} else {
		log.Println("Listening for http websocket far end...")
		err = http.ListenAndServe(address, nil)
	}
	if err != nil {
		log.Fatal(err)
	}

}


// Catches yamux connecting to us
func listenForClients(address string, certificate string) {
	log.Println("Listening for the far end")

	cer, err := tls.LoadX509KeyPair(certificate+".crt", certificate+".key")

    if err != nil {
        log.Println(err)
        return
    }
    config := &tls.Config{Certificates: []tls.Certificate{cer}}

    if config.DynamicRecordSizingDisabled {
		log.Println("Cert")
	}

	//ln, err := net.Listen("tcp", address)
	ln, err := tls.Listen("tcp", address, config)
	if err != nil {
		return
	}
	for {
		conn, err := ln.Accept()
		conn.RemoteAddr()
		log.Printf("Got a SSL connection from %v: ",conn.RemoteAddr())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Errors accepting!")
		}


		reader := bufio.NewReader(conn)

		//read only 64 bytes with timeout=1-3 sec. So we haven't delay with browsers
		conn.SetReadDeadline(time.Now().Add(proxytout))
		statusb := make([]byte,64)
		_,_ = io.ReadFull(reader,statusb)

		//Alternatively  - read all bytes with timeout=1-3 sec. So we have delay with browsers, but get all GET request
		//conn.SetReadDeadline(time.Now().Add(proxytout))
		//statusb,_ := ioutil.ReadAll(magicBuf)

		//log.Printf("magic bytes: %v",statusb[:6])
		//if hex.EncodeToString(statusb) != magicbytes {
		if string(statusb)[:len(agentpassword)] != agentpassword {
			//do HTTP checks
			log.Printf("Received request: %v",string(statusb[:64]))
			status := string(statusb)
			if (strings.Contains(status," HTTP/1.1")){
				httpresonse := "HTTP/1.1 301 Moved Permanently"+
					"\r\nContent-Type: text/html; charset=UTF-8"+
					"\r\nServer: nginx/1.14.1"+
					"\r\nContent-Length: 0"+
					"\r\nConnection: close"+
					"\r\nLocation: "+rurl+
					"\r\n\r\n"

				conn.Write([]byte(httpresonse))
				conn.Close()
			} else {
				conn.Close()
			}

		}else {
			//magic bytes received.
			//disable socket read timeouts
			log.Println("Got remote Client")
			conn.SetReadDeadline(time.Now().Add(100 * time.Hour))


				//connect with yamux
				//Add connection to yamux
				yconf := yamux.DefaultConfig()
				//yconf.EnableKeepAlive = false
				yconf.KeepAliveInterval =  time.Millisecond * 50000
			session, err = yamux.Client(conn, yconf)

		}
	}
}

// Catches clients and connects to yamux
func listenForSocks(address string) error {
	log.Println("Waiting for socks clients")
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		// TODO dial socks5 through yamux and connect to conn

		if session == nil {
			conn.Close()
			continue
		}
		log.Println("Got a socks client")

		log.Println("Opening a stream")
		stream, err := session.Open()


			if err != nil {
				return err
			}


		// connect both of conn and stream

		go func() {
			log.Printf("Starting to copy conn to stream id:  ")


			io.Copy(conn, stream)

			conn.Close()
		}()
		go func() {
			log.Println("Starting to copy stream to conn")

				io.Copy(stream, conn)
				//log.Printf("Closing stream id: %d ",stream)
				stream.Close()

			log.Println("Done copying stream to conn")
		}()
	}
}
