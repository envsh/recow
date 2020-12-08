package main

import (
	"errors"
	"gopp"
	"log"
	"math/rand"
	"net"
	"strings"

	lru "github.com/hashicorp/golang-lru"
)

type DNSDig struct {
	ipver int
	rescc *lru.ARCCache // domain => ipaddr array, string => []string
}

func newDNSDig() *DNSDig {
	this := &DNSDig{}
	this.ipver = IPv4
	rescc, err := lru.NewARC(65536)
	gopp.ErrPrint(err)
	this.rescc = rescc

	return this
}

func (this *DNSDig) Lookup(host string) (addr string, err error) {
	if addrsx, ok := this.rescc.Get(host); ok {
		addr = this.getone(addrsx.([]string))
		return
	}

	addrs, err := net.LookupHost(host)
	gopp.ErrPrint(err, host, addrs)
	if err != nil {
		return
	}
	oldaddrs := addrs
	addrs = this.filterip(addrs)
	if len(addrs) == 0 {
		log.Println("ERR lookup empty", host, oldaddrs)
		err = errors.New("lookup empty")
		return
	}
	addr = this.getone(addrs)
	this.rescc.Add(host, addrs)
	return
}

func (this *DNSDig) filterip(addrs []string) []string {
	if this.ipver == IP46 {
		return addrs
	}

	newlst := []string{}
	for _, addr := range addrs {
		if this.ipver == IPv4 {
			if strings.Contains(addr, ":") { // ipv6
			} else {
				newlst = append(newlst, addr)
			}
		} else if this.ipver == IPv6 {
			if strings.Contains(addr, ":") { // ipv6
				newlst = append(newlst, addr)
			} else {
			}
		}
	}
	return newlst
}

func (this *DNSDig) getone(addrs []string) string {
	idx := rand.Intn(len(addrs))
	return addrs[idx]
}

func (this *DNSDig) CCSize() int {
	return this.rescc.Len()
}

const (
	IPv4 = 1
	IPv6 = 2
	IP46 = 3
)
