package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

type bytePool struct{
	pool sync.Pool
}

func newBytePool() *bytePool{
	return &bytePool{
		pool: sync.Pool{
			New: func() any {
				b:= make([]byte,32*1042)
				return b
			},
		},
	}
}

func (bp *bytePool) Get() []byte{
	return bp.pool.Get().([]byte)
}

func (bp *bytePool) Put(b []byte){
	bp.pool.Put(b)
}



func NewReverseProxy(targetAddr string) (http.Handler , error){
	target,err:= url.Parse(targetAddr)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	proxy.BufferPool = newBytePool()

	return proxy, nil
}