package main

import (
	"bufio"
	"fmt"
	"gopp"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"gopkg.in/ini.v1"
)

type upstream struct {
	uo  *url.URL
	typ string // HTTP/SOCK(4/5)
}

type Recow struct {
	cfgdir     string
	rcfile     string
	directfile string
	proxyfile  string
	logfile    string
	lsner      net.Listener

	ups map[string]*upstream // urlobj =>

	lber interface{}
}

var recow = &Recow{ups: map[string]*upstream{}}

func (this *Recow) init() error {
	this.cfgdir = os.Getenv("HOME") + "/.cow"
	this.rcfile = this.cfgdir + "/rc"
	this.directfile = this.cfgdir + "/direct"
	this.proxyfile = this.cfgdir + "/proxy"

	cfg, err := ini.ShadowLoad(this.rcfile)
	gopp.ErrPrint(err, this.rcfile)
	topsec := cfg.Section("")
	names := topsec.KeyStrings()
	for _, name := range names {
		vals := topsec.Key(name).ValueWithShadows()
		log.Println(name, vals)
		switch name {
		case "listen":
			uo, err := url.Parse(vals[0])
			gopp.ErrPrint(err)
			// portstr := strings.Split(uo.Host, ":")[1]
			uo.Host = "0.0.0.0:8050"
			this.lsner, err = net.Listen("tcp", uo.Host)
			gopp.ErrPrint(err)
			log.Println("Listen on", uo.Host)
		case "loadBalance":
		case "loadBalance2":
		case "logFile":
		case "judgeByIP":
		case "proxy":
			for _, val := range vals {
				uo, err := url.Parse(val)
				gopp.ErrPrint(err, val)
				this.ups[val] = &upstream{uo, uo.Scheme}
			}
		}
	}

	return nil
}

func (this *Recow) serve() error {
	for {
		c, err := this.lsner.Accept()
		gopp.ErrPrint(err)
		if err != nil {
			return err
		}
		go this.dotop(c)
	}
	return nil
}

type pcontext struct {
	cc  net.Conn
	req *http.Request
	upc net.Conn
}

func (this *Recow) dotop(c net.Conn) error {
	reader := bufio.NewReader(c)
	r, err := http.ReadRequest(reader)
	gopp.ErrPrint(err)
	log.Println(r.Method, r.URL, r.Header)
	domain := strings.Split(r.URL.Host, ":")[0]
	ipaddr, err := LookupHost2(domain)
	gopp.ErrPrint(err, r.URL.Host)

	pctx := &pcontext{c, r, nil}
	if canDirect2(ipaddr) {
		log.Println("DIRECT", r.URL)
		if r.Method == http.MethodConnect {
			err = this.dodirsec(pctx)
		} else {
			err = this.dodirtxt(pctx)
		}
	} else {
		log.Println("PROXY", r.URL)
		if r.Method == http.MethodConnect {
			err = this.dopxysec(pctx)
		} else {
			err = this.dopxytxt(pctx)
		}
	}
	return err
}

func (this *Recow) dodirsec(pctx *pcontext) error {
	c, r := pctx.cc, pctx.req

	c.Write([]byte("HTTP/1.1 200 tun estab\r\n\r\n"))
	upc, err := net.Dial("tcp", r.URL.Host)
	gopp.ErrPrint(err)
	go io.Copy(c, upc)
	go io.Copy(upc, c)

	return nil
}

func (this *Recow) dodirtxt(pctx *pcontext) error {
	c, r := pctx.cc, pctx.req

	rehost := r.URL.Host + ":80"
	upc, err := net.Dial("tcp", rehost)
	gopp.ErrPrint(err, rehost)
	reqstr := fmt.Sprintf("%s %s HTTP/1.1\r\n", r.Method, r.URL.Path)
	reqstr += fmt.Sprintf("Host: %s\r\n", r.URL.Host)
	for key, hline := range r.Header {
		if key == "Proxy-Connection" {
			continue
		}
		if key == "User-Agent" {
			reqstr += fmt.Sprintf("%s: %s\r\n", key, hline[0])
		} else {
			reqstr += fmt.Sprintf("%s: %s\r\n", key, hline[0])
		}
	}
	reqstr += fmt.Sprintf("\r\n")
	log.Print("> " + strings.Replace(reqstr, "\n", "\n> ", -1))
	wn, err := upc.Write([]byte(reqstr))
	log.Println(">", wn, rehost)

	go io.Copy(c, upc)
	go io.Copy(upc, c)

	return nil
}

func (this *Recow) dopxysec(pctx *pcontext) error {
	c, r := pctx.cc, pctx.req

	upc, err := net.Dial("tcp", "127.0.0.1:8889")
	gopp.ErrPrint(err)

	fwdreq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n",
		r.URL.Host, r.URL.Host)
	upc.Write([]byte(fwdreq))
	go io.Copy(c, upc)
	go io.Copy(upc, c)

	return nil
}
func (this *Recow) dopxytxt(pctx *pcontext) error {
	c, r := pctx.cc, pctx.req

	upc, err := net.Dial("tcp", "127.0.0.1:8889")
	gopp.ErrPrint(err)

	// 还原请求为字符串
	reqstr := fmt.Sprintf("%s %s HTTP/1.1\r\n", r.Method, r.URL.String())
	reqstr += fmt.Sprintf("Host: %s\r\n", r.URL.Host)
	for key, hline := range r.Header {
		if key == "User-Agent" {
			reqstr += fmt.Sprintf("%s: %s/recow\r\n", key, hline[0])
		} else {
			reqstr += fmt.Sprintf("%s: %s\r\n", key, hline[0])
		}
	}
	reqstr += fmt.Sprintf("\r\n")
	log.Println(reqstr)
	upc.Write([]byte(reqstr))

	go io.Copy(c, upc)
	go io.Copy(upc, c)

	return nil
}

func main() {
	recow.init()
	recow.serve()
}
