package main

import (
	"bufio"
	"fmt"
	"gopp"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	rercfile   string // recow defined rc
	directfile string
	proxyfile  string
	logfile    string
	geoipfile  string               // geoipv1 country database, /usr/share/GeoIP/GeoIP.dat
	ups        map[string]*upstream // urlobj =>
	directed   map[string]bool      // domain =>
	blocked    map[string]bool      // domain =>

	lsner net.Listener

	lber balancer
	dd   *DNSDig

	// stats
	concnt int
	upsize int64
	dlsize int64
}

var recow = &Recow{ups: map[string]*upstream{}}

func (this *Recow) init() error {
	this.directed = map[string]bool{}
	this.blocked = map[string]bool{}
	this.cfgdir = os.Getenv("HOME") + "/.cow"
	this.rcfile = this.cfgdir + "/rc"
	this.directfile = this.cfgdir + "/direct"
	this.proxyfile = this.cfgdir + "/proxy"

	this.lber = newBackupBalance()

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

			// cow 做了配置项名字检查，无法直接重用现有的配置文件，还是需要新的配置文件名字
		case "loadBalance2":
			switch vals[0] {
			case "backup":
				this.lber = newBackupBalance()
			case "random":
			case "hash":
			}
		case "logFile":
		case "geoipFile":
			this.geoipfile = vals[0]
			geoipfile = vals[0]
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
		case "dns": // TODO
			// udp://127.0.0.1:53
			// tcp://127.0.0.1:53
			// tls://127.0.0.1:53
			// doh://127.0.0.1:53
		case "ipv6":
		case "ipv6only":
		}
	}
	this.parseConfig()

	// simple config check
	if this.geoipfile == "" {
		log.Println("WARN geoipFile not set")
	}
	if len(this.ups) == 0 {
		log.Println("WARN upstream proxy(s) not set")
	}

	this.dd = newDNSDig()
	return nil
}

func (this *Recow) parseConfig() {
	this.parserc()
	this.parseDirected()
	this.parseBlocked()

	for item, _ := range this.blocked {
		if _, ok := this.directed[item]; ok {
			log.Println("WARN both direct & proxy contains", item)
		}
	}
}
func (this *Recow) parserc() {

}
func (this *Recow) parseDirected() {
	bcc, err := ioutil.ReadFile(this.directfile)
	gopp.ErrPrint(err)
	lines := strings.Split(string(bcc), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		this.directed[line] = true
	}
}
func (this *Recow) parseBlocked() {
	bcc, err := ioutil.ReadFile(this.proxyfile)
	gopp.ErrPrint(err)
	lines := strings.Split(string(bcc), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		this.blocked[line] = true
	}
}

func domainMatch(lst map[string]bool, domain string) bool {
	fields := strings.Split(domain, ".")
	// log.Println(domain, "match", len(lst), len(fields), fields)
	matched := false
	for i := 0; i < len(fields); i++ {
		if len(fields[i:]) <= 1 {
			break
		}
		subdm := strings.Join(fields[i:], ".")
		_, ok := lst[subdm]
		// log.Println(domain, subdm, ok, len(lst))
		if ok {
			matched = true
			break
		}
	}
	// log.Println("matched", domain, matched, len(lst))
	return matched
}

func (this *Recow) canDirect(domain string, ipaddr string) bool {
	blocked := domainMatch(this.blocked, domain)
	if blocked {
		return false
	}

	directed := domainMatch(this.directed, domain)
	if directed {
		return true
	}

	candirip := canDirect2(ipaddr)
	return candirip
}

func (this *Recow) serve() error {
	for {
		c, err := this.lsner.Accept()
		gopp.ErrPrint(err)
		if err != nil {
			return err
		}
		go func() {
			err := this.dotop(c)
			if false {
				gopp.ErrPrint(err)
			}
		}()
	}
	return nil
}

//safe shutdown conn
type ssconn struct {
	net.Conn
	closed atomic.Value
}

func newssconn(c net.Conn) *ssconn {
	this := &ssconn{Conn: c}
	this.closed.Store(false)
	return this
}
func (this *ssconn) shutdown() error {
	if this.closed.Load().(bool) {
		return nil
	}
	this.closed.Store(true)
	return this.Conn.Close()
}

type pcontext struct {
	cc       net.Conn
	scc      *ssconn
	req      *http.Request
	upc      net.Conn
	supc     *ssconn
	retry    int
	btime    time.Time
	donetm12 time.Time // io.Copy(c1,c2) finish time
	donetm21 time.Time // io.Copy(c2,c1) finish time
	xchlen12 int64
	xchlen21 int64
}

func newpcontext(c net.Conn, r *http.Request) *pcontext {
	this := &pcontext{cc: c, req: r}
	this.scc = newssconn(c)
	this.btime = time.Now()
	return this
}
func (this *pcontext) istimeouted() bool {
	return time.Since(this.btime) > 15*time.Second
}
func (this *pcontext) since() time.Duration {
	return time.Since(this.btime)
}

func (this *pcontext) connectpxyup(lber balancer) error {
	defer func() { this.retry++ }()
	itemx := lber.Sel(this.retry)
	item := itemx.(string)
	uo, err := url.Parse(item)
	gopp.ErrPrint(err)
	c, err := net.Dial("tcp", uo.Host)
	this.setupc(c)
	return err
}

func (this *pcontext) setupc(c net.Conn) {
	if c == nil {
		return
	}
	if this.upc != nil {
		log.Println("upc not nil???", this.upc, this.supc)
		if this.supc != nil {
			this.supc.shutdown()
		}
	}
	this.upc = c
	this.supc = newssconn(c)
}

// ensure have port part
func ensureHostport(scheme string, host string) string {
	if strings.Index(host, ":") > 0 {
		return host
	}
	if scheme == "https" {
		return host + ":443"
	}
	if scheme == "http" {
		return host + ":80"
	}
	log.Println("wtt not supported", scheme, host)
	return host
}
func (this *pcontext) connectdirup() error {
	defer func() { this.retry++ }()
	r := this.req
	rehost := ensureHostport(r.URL.Scheme, r.URL.Host)
	upc, err := net.Dial("tcp", rehost)
	gopp.ErrPrint(err)
	this.setupc(upc)
	return err
}
func (this *pcontext) connectdirup2(ipaddr string) error {
	defer func() { this.retry++ }()
	r := this.req
	rehost := ensureHostport(r.URL.Scheme, r.URL.Host)
	arr := strings.Split(rehost, ":")
	rehost = fmt.Sprintf("%s:%s", ipaddr, arr[1])
	upc, err := net.Dial("tcp", rehost)
	gopp.ErrPrint(err)
	this.setupc(upc)
	return err
}
func (this *pcontext) cleanup() {
	this.scc.shutdown()
	if this.supc != nil {
		this.supc.shutdown()
	}
	this.req.Body.Close()
}

func (this *Recow) dotop(c net.Conn) error {
	//defer c.Close()
	reader := bufio.NewReader(c)
	r, err := http.ReadRequest(reader)
	bufed := reader.Buffered()
	if err != nil {
		bcc := make([]byte, bufed)
		reader.Read(bcc)
		gopp.ErrPrint(err, "ReadRequest error", bufed, gopp.SubStr(string(bcc), 64))
	}
	if err != nil {
		c.Close()
		return fmt.Errorf("ReadRequest error %v %v", bufed, err)
	}
	pctx := newpcontext(c, r)
	defer pctx.cleanup()
	//log.Println(r.Method, r.URL, gopp.MapKeys(r.Header), r.ContentLength)

	domain := strings.Split(r.URL.Host, ":")[0]
	// TODO check ADBlock by domain

	// ipaddr, err := LookupHost2(domain)
	ipaddr, err := this.dd.Lookup(domain)
	gopp.ErrPrint(err, r.URL.Host, "cclen", this.dd.CCSize())
	if err != nil {
		return err
	}

	// candir := canDirect2(ipaddr)
	candir := this.canDirect(domain, ipaddr)
	for {
		if pctx.istimeouted() {
			break
		}
		if candir {
			err = pctx.connectdirup2(ipaddr)
			gopp.ErrPrint(err, r.URL.Host)
		} else {
			err = pctx.connectpxyup(this.lber)
			gopp.ErrPrint(err, r.URL.Host, pctx.retry, this.lber.Len())
		}
		if err != nil && gopp.ErrHave(err, "connection refused") {
			break
		}
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		if candir {
			log.Println("DIRECT", r.Method, r.URL, pctx.upc.RemoteAddr())
			if r.Method == http.MethodConnect {
				err = this.dodirsec(pctx)
			} else {
				err = this.dodirtxt(pctx)
			}
		} else {
			log.Println("PROXY", r.Method, r.URL, pctx.upc.RemoteAddr(), pctx.retry)
			if r.Method == http.MethodConnect {
				err = this.dopxysec(pctx)
			} else {
				err = this.dopxytxt(pctx)
			}
		}
		break
	}

	this.updatestats(pctx)
	log.Println("release", r.Method, r.URL, "cclen", r.ContentLength,
		"DL", pctx.xchlen12, "UP", pctx.xchlen21,
		"loclink", gopp.Dur2hum(pctx.donetm12.Sub(pctx.btime)),
		"remlink", gopp.Dur2hum(pctx.donetm21.Sub(pctx.btime)), err)
	return err
}

func (this *Recow) updatestats(pctx *pcontext) {
	this.concnt++
	this.dlsize += pctx.xchlen12
	this.upsize += pctx.xchlen21
}

func iobicopy(pctx *pcontext, c1 net.Conn, c2 net.Conn) (err12 error, err21 error) {
	var resch = make(chan error, 2)
	go func() {
		xn, err := io.Copy(c1, c2)
		err12 = err
		pctx.xchlen12 = xn
		pctx.donetm12 = time.Now()
		resch <- err
	}()
	go func() {
		xn, err := io.Copy(c2, c1)
		err21 = err
		pctx.xchlen21 = xn
		pctx.donetm21 = time.Now()
		resch <- err
	}()

	err0 := <-resch
	select {
	case <-resch:
	case <-time.After(5 * time.Second):
		pctx.scc.shutdown()
		pctx.supc.shutdown()
		<-resch
	}
	if err0 == nil {
		err12 = nil
		err21 = nil
	}

	return
}

func (this *Recow) dodirsec(pctx *pcontext) error {
	c, r, upc := pctx.cc, pctx.req, pctx.upc
	_ = r

	wn, err := c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	gopp.ErrPrint(err, wn)

	// go io.Copy(c, upc)
	// go io.Copy(upc, c)
	err12, err21 := iobicopy(pctx, c, upc)
	if err12 != nil {
		err = err12
	}
	if err21 != nil {
		err = err21
	}

	return err
}

func (this *Recow) dodirtxt(pctx *pcontext) error {
	c, r, upc := pctx.cc, pctx.req, pctx.upc
	_ = r

	reqstr := fmt.Sprintf("%s %s HTTP/1.1\r\n", r.Method, r.URL.Path)
	reqstr += rawEncodeHeader(filterHeader(r.Header), r.URL.Host)
	reqstr += fmt.Sprintf("\r\n")
	log.Print("> " + strings.Replace(reqstr, "\n", "\n> ", -1))
	wn, err := upc.Write([]byte(reqstr))
	log.Println(">", wn, ensureHostport(r.URL.Scheme, r.URL.Host))

	// go io.Copy(c, upc)
	// go io.Copy(upc, c)
	err12, err21 := iobicopy(pctx, c, upc)
	if err12 != nil {
		err = err12
	}
	if err21 != nil {
		err = err21
	}

	return err
}

func (this *Recow) dopxysec(pctx *pcontext) error {
	c, r, upc := pctx.cc, pctx.req, pctx.upc

	fwdreq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n",
		r.URL.Host, r.URL.Host)
	_, err := upc.Write([]byte(fwdreq))

	// go io.Copy(c, upc)
	// go io.Copy(upc, c)
	err12, err21 := iobicopy(pctx, c, upc)
	if err12 != nil {
		err = err12
	}
	if err21 != nil {
		err = err21
	}

	return err
}
func (this *Recow) dopxytxt(pctx *pcontext) error {
	c, r, upc := pctx.cc, pctx.req, pctx.upc

	// 还原请求为字符串
	reqstr := fmt.Sprintf("%s %s HTTP/1.1\r\n", r.Method, r.URL.String())
	reqstr += rawEncodeHeader(filterHeader(r.Header), r.URL.Host)
	reqstr += fmt.Sprintf("\r\n")
	log.Println(reqstr)
	_, err := upc.Write([]byte(reqstr))

	// go io.Copy(c, upc)
	// go io.Copy(upc, c)
	err12, err21 := iobicopy(pctx, c, upc)
	if err12 != nil {
		err = err12
	}
	if err21 != nil {
		err = err21
	}

	return err
}

func filterHeader(headers http.Header) http.Header {
	var newhdrs = http.Header{}
	for key, hline := range headers {
		switch strings.ToLower(key) {
		case "proxy-connect":
		case "user-agent": // shorten UA
			newhdrs[key] = []string{fmt.Sprintf("firefox %2drc", rand.Intn(50)+50)}
		default:
			newhdrs[key] = hline
		}
	}
	return newhdrs
}

// \r\n seperated string
func rawEncodeHeader(headers http.Header, host string) string {
	str := ""
	hashost := false
	for key, hline := range headers {
		if strings.ToLower(key) == "host" && host != "" {
			hashost = true
			str += fmt.Sprintf("%s: %s\r\n", key, host)
		} else {
			str += fmt.Sprintf("%s: %s\r\n", key, hline[0])
		}
	}
	if !hashost && host != "" {
		str += fmt.Sprintf("Host: %s\r\n", host)
	}
	return str
}

func main() {
	recow.init()
	recow.serve()
}
