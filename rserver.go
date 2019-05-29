package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"crypto/tls"
	"regexp"

	"time"
	"bufio"
	//"encoding/hex"
	"github.com/hashicorp/yamux"
	"strings"

)
//var session *yamux.Session
var stream *yamux.Stream
var proxytout = time.Millisecond * 2000 //timeout for wait for password
var rurl string//redirect URL

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
		statusb := make([]byte,160)
		_,_ = io.ReadFull(reader,statusb)

		//Alternatively  - read all bytes with timeout=1-3 sec. So we have delay with browsers, but get all GET request
		//conn.SetReadDeadline(time.Now().Add(proxytout))
		//statusb,_ := ioutil.ReadAll(magicBuf)

		//log.Printf("magic bytes: %v",statusb[:6])
		//if hex.EncodeToString(statusb) != magicbytes {

		r, _ := regexp.Compile("Xauth:.*")


		auth_header := r.FindString(string(statusb))
		if len(auth_header) > 7 {
			auth_header = auth_header[7:]
		}else{
			auth_header = ""
		}
		//log.Printf("Found header: %s",auth_header)

		//if string(statusb)[:len(agentpassword)] != agentpassword {
		if (len(auth_header) >= len(agentpassword)) && (auth_header[:len(agentpassword)] == agentpassword) {
			//pass is correct
			//disable socket read timeouts
			log.Println("Got remote Client")
			httpresonse := "HTTP/1.1 200 OK"+
				"\r\nContent-Type: text/html; charset=UTF-8"+
				"\r\nServer: nginx/1.14.1"+
				"\r\nContent-Length: 0"+
				"\r\nConnection: keep-alive"+
				"\r\n\r\n"

			conn.Write([]byte(httpresonse))
			conn.SetReadDeadline(time.Now().Add(100 * time.Hour))


			//connect with yamux
			//Add connection to yamux
			//create default yamux config
			yconf := yamux.DefaultConfig()
			//yconf.EnableKeepAlive = false
			//set yamux keepalives
			yconf.KeepAliveInterval =  time.Millisecond * 50000
			session, err = yamux.Client(conn, yconf)

		}else {
			//do HTTP checks
			log.Printf("Received request: %v",string(statusb[:160]))
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
