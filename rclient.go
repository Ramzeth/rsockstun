package main

import (
	"errors"
	"fmt"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/websocket"
	"log"
	"net"
	"crypto/tls"

	socks5 "github.com/armon/go-socks5"
	"github.com/hashicorp/yamux"
	"encoding/base64"
	"time"
	"net/http"
	"bufio"
	"strings"
	"github.com/ThomsonReutersEikon/go-ntlm/ntlm"
	"io/ioutil"
)


var encBase64 = base64.StdEncoding.EncodeToString
var decBase64 = base64.StdEncoding.DecodeString
var username string
var domain string
var password string
var connectproxystring string
var useragent string
var proxytimeout = time.Millisecond * 1000 //timeout for proxyserver response

func makeDoTQuery(dnsName string) ([]byte, error) {
	query := dnsmessage.Message{
		Header: dnsmessage.Header{
			RecursionDesired: true,
		},
		Questions: []dnsmessage.Question{
			{
				Name:  dnsmessage.MustNewName(dnsName),
				Type:  dnsmessage.TypeTXT,
				Class: dnsmessage.ClassINET,
			},
		},
	}
	req, err := query.Pack()
	if err != nil {
		return nil, err
	}
	l := len(req)
	req = append([]byte{
		uint8(l >> 8),
		uint8(l),
	}, req...)
	return req, nil
}

func parseTXTResponse(buf []byte, wantName string) (string, error) {
	var p dnsmessage.Parser
	hdr, err := p.Start(buf)
	if err != nil {
		return "", err
	}
	if hdr.RCode != dnsmessage.RCodeSuccess {
		return "", fmt.Errorf("DNS query failed, rcode=%s", hdr.RCode)
	}
	if err := p.SkipAllQuestions(); err != nil {
		return "", err
	}
	for {
		h, err := p.AnswerHeader()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			return "", err
		}
		if h.Type != dnsmessage.TypeTXT || h.Class != dnsmessage.ClassINET {
			continue
		}
		if !strings.EqualFold(h.Name.String(), wantName) {
			if err := p.SkipAnswer(); err != nil {
				return "", err
			}
		}
		r, err := p.TXTResource()
		if err != nil {
			return "", err
		}
		return r.TXT[0], nil
	}
	return "", errors.New("No TXT record found")
}

func QueryESNIKeysForHost(hostname string) ([]byte, error) {
	esniDnsName := "_esni." + hostname + "."
	query, err := makeDoTQuery(esniDnsName)
	if err != nil {
		return nil, fmt.Errorf("Building DNS query failed: %s", err)
	}
	tlsconfig := &tls.Config{InsecureSkipVerify: true,}
	c, err := tls.Dial("tcp", "1.1.1.1:853", tlsconfig)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	// Send DNS query
	n, err := c.Write(query)
	if err != nil || n != len(query) {
		return nil, fmt.Errorf("Failed to write query: %s", err)
	}

	// Read DNS response
	buf := make([]byte, 4096)
	n, err = c.Read(buf)
	if n < 2 && err != nil {
		return nil, fmt.Errorf("Cannot read response: %s", err)
	}
	txt, err := parseTXTResponse(buf[2:n], esniDnsName)
	if err != nil {
		return nil, fmt.Errorf("Cannot process TXT record: %s", err)
	}
	return base64.StdEncoding.DecodeString(txt)
}

func connectForWsSocks(address string, proxy string, frontDomain string) error {
	//create socks5 server
	server, err := socks5.New(&socks5.Config{})
	if err != nil {
		return err
	}

	//replace in address ws: -> ws:// and wss: -> wss://
	address = strings.Replace(address,"ws:","ws://",1)
	address = strings.Replace(address,"wss:","wss://",1)
	//create ws config
	wsconf, _:= websocket.NewConfig(address, address)

	//set server name for eSNI domain fronting
	wsconf.TlsConfig = &tls.Config{InsecureSkipVerify: true, FakeServerName: frontDomain}

	//esniKeysBytes, err := QueryESNIKeysForHost("www.cloudflare.com")
	esniKeysBytes, err := QueryESNIKeysForHost(strings.Split((strings.Split(address,"://")[1]),":")[0])

	wsconf.TlsConfig.ClientESNIKeys, err = tls.ParseESNIKeys(esniKeysBytes)

	if wsconf.TlsConfig.ClientESNIKeys == nil {
		log.Fatalf("Failed to process ESNI response for host: %s", err)
	}

	log.Println("Dialing Web Socket")
	wsconn, err := websocket.DialConfig(wsconf)
	if err != nil {
		// handle error
		log.Printf("Error when dialing webSocket, %v",err)
		return err
	}
	defer wsconn.Close()

	wsconn.PayloadType = websocket.BinaryFrame

	log.Println("Starting client")
	wsconn.Write([]byte(agentpassword))
	time.Sleep(time.Second * 2)
	session, err = yamux.Server(wsconn, nil)
	if err != nil {
		return err
	}

	for {
		stream, err := session.Accept()
		log.Println("Acceping stream")
		if err != nil {
			return err
		}
		log.Println("Passing off to socks5")
		go func() {
			err = server.ServeConn(stream)
			if err != nil {
				log.Println(err)
			}
		}()
	}

	return nil
}


func connectviaproxy(proxyaddr string, connectaddr string) net.Conn {

	if (username != "") && (password != "") && (domain != "") {
		connectproxystring = "CONNECT " + connectaddr + " HTTP/1.1" + "\r\nHost: " + connectaddr +
			"\r\nUser-Agent: "+useragent+
			"\r\nProxy-Authorization: NTLM TlRMTVNTUAABAAAABoIIAAAAAAAAAAAAAAAAAAAAAAA=" +
			"\r\nProxy-Connection: Keep-Alive" +
			"\r\n\r\n"

	}else	{
		connectproxystring = "CONNECT " + connectaddr + " HTTP/1.1" + "\r\nHost: " + connectaddr +
			"\r\nUser-Agent: "+useragent+
			"\r\nProxy-Connection: Keep-Alive" +
			"\r\n\r\n"
	}

	//log.Print(connectproxystring)

	conn, err := net.Dial("tcp", proxyaddr)
	if err != nil {
		// handle error
		log.Printf("Error connect: %v",err)
	}
	conn.Write([]byte(connectproxystring))

	time.Sleep(proxytimeout) //Because socket does not close - we need to sleep for full response from proxy

	resp,err := http.ReadResponse(bufio.NewReader(conn),&http.Request{Method: "CONNECT"})
	status := resp.Status

	//log.Print(status)
	//log.Print(resp)

	if (resp.StatusCode == 200)  || (strings.Contains(status,"HTTP/1.1 200 ")) ||
		(strings.Contains(status,"HTTP/1.0 200 ")){
		log.Print("Connected via proxy. No auth required")
		return conn
	}

	if (strings.Contains(status,"407 Proxy Authentication Required")){
		log.Print("Got Proxy auth:")
		ntlmchall := resp.Header.Get("Proxy-Authenticate")
		if ntlmchall != "" {
			log.Print("Got NTLM challenge:")
			//log.Print(ntlmchall)
			var session ntlm.ClientSession
			session, _ = ntlm.CreateClientSession(ntlm.Version2, ntlm.ConnectionlessMode)
			session.SetUserInfo(username,password,domain)
			//negotiate, _ := session.GenerateNegotiateMessage()
			//log.Print(negotiate)

			ntlmchall = ntlmchall[5:]
			ntlmchallb,_ := decBase64(ntlmchall)


			challenge, _ := ntlm.ParseChallengeMessage(ntlmchallb)
			session.ProcessChallengeMessage(challenge)
			authenticate, _ := session.GenerateAuthenticateMessage()
			ntlmauth:= encBase64(authenticate.Bytes())

			//log.Print(authenticate)
			connectproxystring = "CONNECT "+connectaddr+" HTTP/1.1"+"\r\nHost: "+connectaddr+
				"\r\nUser-Agent: Mozilla/5.0 (Windows NT 6.1; Trident/7.0; rv:11.0) like Gecko"+
				"\r\nProxy-Authorization: NTLM "+ntlmauth+
				"\r\nProxy-Connection: Keep-Alive"+
				"\r\n\r\n"


			//Empty read buffer
			/*
			var statusb []byte
			//conn.SetReadDeadline(time.Now().Add(1000 * time.Millisecond))
			bufReader := bufio.NewReader(conn)
			n, err := bufReader.Read(statusb)
			//statusb,_ := ioutil.ReadAll(bufReader)
			if err != nil {
				if err == io.EOF {
					log.Printf("Readed %v vites",n)
				}
			}
			status = string(statusb)
			*/

			conn.Write([]byte(connectproxystring))

			//read response
			bufReader := bufio.NewReader(conn)
			conn.SetReadDeadline(time.Now().Add(proxytimeout))
			statusb,_ := ioutil.ReadAll(bufReader)

			status = string(statusb)

			//disable socket read timeouts
			conn.SetReadDeadline(time.Now().Add(100 * time.Hour))

			if (strings.Contains(status,"HTTP/1.1 200 ")){
				log.Print("Connected via proxy")
				return conn
			} else{
				log.Printf("Not Connected via proxy. Status:%v",status)
				return nil
			}
		}

	}else {
		log.Print("Not connected via proxy")
		conn.Close()
		return nil
	}

	return conn
}

func connectForSocks(address string, proxy string) error {
	server, err := socks5.New(&socks5.Config{})
	if err != nil {
		return err
	}

	conf := &tls.Config{
         InsecureSkipVerify: true,
    }

	var conn net.Conn
	var connp net.Conn
	var newconn net.Conn
	//var conntls tls.Conn
	//var conn tls.Conn
	if proxy == "" {
		log.Println("Connecting to far end")
		//conn, err = net.Dial("tcp", address)
		conn, err = tls.Dial("tcp", address, conf)
		if err != nil {
			return err
		}
	}else {
		log.Println("Connecting to proxy ...")
		connp = connectviaproxy(proxy,address)
		if connp != nil{
			log.Println("Proxy successfull. Connecting to far end")
			conntls := tls.Client(connp,conf)
			err := conntls.Handshake()
			if err != nil {
				log.Printf("Error connect: %v",err)
				return err
			}
			newconn = net.Conn(conntls)
		}else{
			log.Println("Proxy NOT successfull. Exiting")
			return nil
		}
	}

	log.Println("Starting client")
	if proxy == "" {
		conn.Write([]byte(agentpassword))
		//time.Sleep(time.Second * 1)
		session, err = yamux.Server(conn, nil)
	}else {

		//log.Print(conntls)
		newconn.Write([]byte(agentpassword))
		time.Sleep(time.Second * 1)
		session, err = yamux.Server(newconn, nil)
	}
	if err != nil {
		return err
	}

	for {
		stream, err := session.Accept()
		log.Println("Acceping stream")
		if err != nil {
			return err
		}
		log.Println("Passing off to socks5")
		go func() {
			err = server.ServeConn(stream)
			if err != nil {
				log.Println(err)
			}
		}()
	}
}
