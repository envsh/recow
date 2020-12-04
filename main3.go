package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"gopp"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/abh/geoip"
	"github.com/miekg/dns"
	"github.com/oschwald/geoip2-golang"
)

func HtcliForProxy(proxyUrl string) *http.Client {
	proxy, _ := url.Parse(proxyUrl)
	tr := &http.Transport{
		Proxy:           http.ProxyURL(proxy),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{
		Transport: tr,
		// Timeout:   time.Second * 5, //超时时间
	}
	return client
}

func HtreqCopy(r *http.Request) *http.Request {
	return r
}

// dns resolve
func LookupHost2(host string) (addr string, err error) {
	addrs, err := net.LookupHost(host)
	gopp.ErrPrint(err)
	log.Println(host, addrs)
	addr = addrs[0]
	return
}
func LookupHost(host string) (addr string, err error) {

	m1 := &dns.Msg{}
	m1.Id = dns.Id()
	m1.RecursionDesired = true
	m1.Question = make([]dns.Question, 1)
	m1.Question[0] = dns.Question{"miek.nl.", dns.TypeA, dns.ClassINET}
	dnscli := &dns.Client{}
	dnscli.Net = "udp"
	in, rtt, err := dnscli.Exchange(m1, "1.1.1.1:53")
	gopp.ErrPrint(err)
	log.Println(in, rtt)
	return
}

var gig *geoip.GeoIP

func canDirect2(ipaddr string) bool {
	file := "/usr/share/GeoIP/GeoIP.dat"

	if gig == nil {
		gi, err := geoip.Open(file)
		if err != nil {
			fmt.Printf("Could not open GeoIP database\n")
		}
		gig = gi
	}

	gi := gig
	if gi != nil {
		country, netmask := gi.GetCountry(ipaddr)
		log.Println(ipaddr, country, netmask)
		if country == "CN" {
			return true
		}
	}
	return false
}

func canDirect(ipaddr string) bool {
	db, err := geoip2.Open("GeoIP2-City.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	// If you are using strings that may be invalid, check that ip is not nil
	ip := net.ParseIP("81.2.69.142")
	record, err := db.City(ip)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Portuguese (BR) city name: %v\n", record.City.Names["pt-BR"])
	if len(record.Subdivisions) > 0 {
		fmt.Printf("English subdivision name: %v\n", record.Subdivisions[0].Names["en"])
	}
	fmt.Printf("Russian country name: %v\n", record.Country.Names["ru"])
	fmt.Printf("ISO country code: %v\n", record.Country.IsoCode)
	fmt.Printf("Time zone: %v\n", record.Location.TimeZone)
	fmt.Printf("Coordinates: %v, %v\n", record.Location.Latitude, record.Location.Longitude)
	return true
}

func main3() {
	if false {
		LookupHost2("zhihu.com")
		LookupHost2("google.com")
		//canDirect("1.1.1.1")
		canDirect2("1.1.1.1")
		return
	}
	lsner, err := net.Listen("tcp", ":8050")
	gopp.ErrPrint(err)
	for {
		c, err := lsner.Accept()
		gopp.ErrPrint(err)

		go func() {
			reader := bufio.NewReader(c)
			r, err := http.ReadRequest(reader)
			gopp.ErrPrint(err)
			log.Println(r.Method, r.URL, r.Header)
			domain := strings.Split(r.URL.Host, ":")[0]
			ipaddr, err := LookupHost2(domain)
			gopp.ErrPrint(err, r.URL.Host)
			if canDirect2(ipaddr) {
				if r.Method == http.MethodConnect {
					c.Write([]byte("HTTP/1.1 200 tun estab\r\n\r\n"))
					upc, err := net.Dial("tcp", r.URL.Host)
					gopp.ErrPrint(err)
					go io.Copy(c, upc)
					go io.Copy(upc, c)
				} else {
					rehost := r.URL.Host + ":80"
					upc, err := net.Dial("tcp", rehost)
					gopp.ErrPrint(err, rehost)
					reqstr := fmt.Sprintf("%s %s HTTP/1.1\r\n", r.Method, r.URL.Path)
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
				}
			} else {
				if r.Method == http.MethodConnect {
					upc, err := net.Dial("tcp", "127.0.0.1:8889")
					gopp.ErrPrint(err)
					fwdreq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n",
						r.URL.Host, r.URL.Host)
					upc.Write([]byte(fwdreq))
					go io.Copy(c, upc)
					go io.Copy(upc, c)
				} else {
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
				}
			}
		}()
	}
}
