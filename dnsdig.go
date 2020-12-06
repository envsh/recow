package main

import (
	"gopp"
	"log"
	"net"

	lru "github.com/hashicorp/golang-lru"
)

type DNSDig struct {
	rescc *lru.ARCCache // host => ipaddr both string
}

func newDNSDig() *DNSDig {
	this := &DNSDig{}
	rescc, err := lru.NewARC(65536)
	gopp.ErrPrint(err)
	this.rescc = rescc

	return this
}

func (this *DNSDig) Lookup(host string) (addr string, err error) {
	if addrx, ok := this.rescc.Get(host); ok {
		addr = addrx.(string)
		return
	}

	addrs, err := net.LookupHost(host)
	gopp.ErrPrint(err)
	log.Println(host, addrs)
	if err != nil {
		return
	}
	addr = addrs[0]

	this.rescc.Add(host, addr)
	return
}
