package main

import (
	"bufio"
	"fmt"
	"gopp"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"gopkg.in/ini.v1"
)

type balancer interface {
	// if retry == 0, then reset backupBalance state
	Sel(retry int) interface{}
	Add(item interface{}, weight int) // duplicate add permitted
	Del(item interface{})
	Len() int
}

type baseBalance struct {
	mu    sync.RWMutex
	items []interface{}
	index int
}

func (this *backupBalance) Add(item interface{}, weight int) {
	this.mu.Lock()
	defer this.mu.Unlock()

	this.items = append(this.items, item)
}
func (this *backupBalance) Del(item interface{}) {
	this.mu.Lock()
	defer this.mu.Unlock()

}
func (this *backupBalance) Len() int {
	this.mu.Lock()
	defer this.mu.Unlock()
	return len(this.items)
}

type firstBalance struct {
}

type backupBalance struct {
	baseBalance
}

func newBackupBalance() *backupBalance {
	this := &backupBalance{}
	return this
}

func (this *backupBalance) Sel(retry int) interface{} {
	this.mu.Lock()
	defer this.mu.Unlock()

	if retry == 0 {
		this.index = 0
	} else {
		this.index++
	}
	return this.items[this.index%len(this.items)]
}

type roundrobinBalance struct {
	baseBalance
}

func (this *roundrobinBalance) Sel(retry int) interface{} {
	this.mu.Lock()
	defer this.mu.Unlock()

	this.index++

	return this.items[this.index%len(this.items)]
}

type randomBalance struct {
	baseBalance
}

func (this *randomBalance) Sel(retry int) interface{} {
	this.mu.Lock()
	defer this.mu.Unlock()

	this.index = rand.Intn(len(this.items) * 100)

	return this.items[this.index%len(this.items)]
}

type hashBalance struct {
	baseBalance
}

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

	lber balancer
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
			switch vals[0] {
			case "backup":
				this.lber = newBackupBalance()
			case "random":
			case "hash":
			}
		case "logFile":
		case "judgeByIP": // default behaviour
		case "proxy":
			for _, val := range vals {
				uo, err := url.Parse(val)
				gopp.ErrPrint(err, val)
				this.ups[val] = &upstream{uo, uo.Scheme}
				if uo.Scheme == "http" {
					this.lber.Add(val, 0)
				}
			}
			log.Println("ups", len(this.ups), "lber", this.lber.Len())
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
	cc    net.Conn
	req   *http.Request
	upc   net.Conn
	retry int
}

func (this *pcontext) connectup(lber balancer) error {
	defer func() { this.retry++ }()
	itemx := lber.Sel(this.retry)
	item := itemx.(string)
	uo, err := url.Parse(item)
	gopp.ErrPrint(err)
	c, err := net.Dial("tcp", uo.Host)
	this.upc = c
	return err
}

func (this *Recow) dotop(c net.Conn) error {
	reader := bufio.NewReader(c)
	r, err := http.ReadRequest(reader)
	gopp.ErrPrint(err)
	log.Println(r.Method, r.URL, r.Header)
	domain := strings.Split(r.URL.Host, ":")[0]
	ipaddr, err := LookupHost2(domain)
	gopp.ErrPrint(err, r.URL.Host)

	pctx := &pcontext{c, r, nil, 0}
	candir := canDirect2(ipaddr)
	for {
		if candir {
			log.Println("DIRECT", r.URL)
			if r.Method == http.MethodConnect {
				err = this.dodirsec(pctx)
			} else {
				err = this.dodirtxt(pctx)
			}
		} else {
			err = pctx.connectup(this.lber)
			gopp.ErrPrint(err, pctx.retry, this.lber.Len())
			if err != nil {
				continue
			}
			log.Println("PROXY", r.URL, pctx.upc.RemoteAddr(), pctx.retry)
			if r.Method == http.MethodConnect {
				err = this.dopxysec(pctx)
			} else {
				err = this.dopxytxt(pctx)
			}
		}
		break
	}
	return err
}

func (this *Recow) dodirsec(pctx *pcontext) error {
	c, r := pctx.cc, pctx.req

	c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
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
	c, r, upc := pctx.cc, pctx.req, pctx.upc

	fwdreq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n",
		r.URL.Host, r.URL.Host)
	upc.Write([]byte(fwdreq))
	go io.Copy(c, upc)
	go io.Copy(upc, c)

	return nil
}
func (this *Recow) dopxytxt(pctx *pcontext) error {
	c, r, upc := pctx.cc, pctx.req, pctx.upc

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
