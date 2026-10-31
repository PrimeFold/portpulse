package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)




type RateLimiter struct{
	timeWindow int
	maxCount int
	ConnAddr string

}

 
func NewRateLimiter(Addr string , limit int , w int) *RateLimiter {
	return &RateLimiter{
		timeWindow: w,
		maxCount: limit,
		ConnAddr: Addr,	
	}
}


func (r *RateLimiter) queryByteCache(command string) (string , error){
	conn,err:= net.Dial("tcp",r.ConnAddr)

	if err != nil{
		log.Printf("Error Dialing the Address [%s] : %v ",r.ConnAddr,err)
		return "",err
	}

	defer conn.Close()


	_,err = fmt.Fprintf(conn,"%s\n",command)
	if err != nil{
		return "",err
	}


	reader:= bufio.NewReader(conn)
	reply,err := reader.ReadString('\n')

	if err != nil{
		return "",err
	}

	return strings.TrimSpace(reply),nil
}


func (r *RateLimiter) Allow(clientIp string) bool{

	key:= fmt.Sprintf("ratelimit:%s",clientIp)

	reply,err:= r.queryByteCache(fmt.Sprintf("GET %s",key))

	if err != nil{
		return true
	}


	//If response comes with an ERR or is BLANK , We send SET command to set the cache for that IP.
	if strings.HasPrefix(reply,"ERR") || reply == ""{
		r.queryByteCache(fmt.Sprintf("SET %s 1 %d",key,r.timeWindow))
	}


	parts:= strings.Fields(reply)

	count,_:= strconv.Atoi(parts[1])

	if count >= r.maxCount{
		return false //BLOCK THE CLIENT AS IT EXCEEDS THE LIMIT
	}

	_,_ = r.queryByteCache(fmt.Sprintf("SET %s %d %d",key,count+1,r.timeWindow))

	return true

}


func (r *RateLimiter) Middleware(next http.Handler) http.Handler{
	return http.HandlerFunc(func(w http.ResponseWriter , req *http.Request){
		// 1. Extract IP (removing port if present, e.g. "127.0.0.1:54321" -> "127.0.0.1")
		ip, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			ip = req.RemoteAddr
		}


		// Checking if the ip is allowed..
		if !r.Allow(ip){
			w.Header().Set("Content-Type","application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}` + "\n"))
			return
		}

		next.ServeHTTP(w,req)
	})
}



