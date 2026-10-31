package main

import (
	"hash/fnv"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// limiterShard represents an independent partition with its own mutex lock
type limiterShard struct {
	mu     sync.RWMutex
	counts map[string]int
	resets map[string]time.Time
}

const numShards = 32

type SharedRateLimiter struct{
	shards [numShards]*limiterShard
	maxCount  int
	windowSec time.Duration
}

func NewSharedRateLimiter(maxCount int , windowSec int) *SharedRateLimiter{
	limiter:= &SharedRateLimiter{
		maxCount : maxCount,
		windowSec : time.Duration(windowSec) * time.Second,
	}
	
	for i := 0; i < numShards; i++ {
		limiter.shards[i] = &limiterShard{
			counts: make(map[string]int),
			resets: make(map[string]time.Time),
		}
	}

	return limiter
}


func (srl *SharedRateLimiter) getShard(key string) (*limiterShard,error){
	//Hash the key to a big 32 bit number..
	h:= fnv.New32a()
	_,err:= h.Write([]byte(key))

	if err != nil{
		return nil,err
	}

	// Bitwise AND with 31 is the ultra-fast equivalent of % 32 for powers of 2!
	shardIdx := h.Sum32() & (numShards - 1)
	return srl.shards[shardIdx],nil

}

// Allow checks if the client IP is under the limit, locking ONLY their specific shard
func (srl *SharedRateLimiter) Allow(clientIP string) bool {
	shard,err:= srl.getShard(clientIP)

	if err != nil{
		log.Printf("Error in getting shard for IP : [%s] : %v",clientIP,err)
	}

	shard.mu.Lock()
	defer shard.mu.Unlock()

	now:= time.Now()


	resetTime,exists:= shard.resets[clientIP]
	if !exists || now.After(resetTime){
		shard.counts[clientIP] = 1
		shard.resets[clientIP] = now.Add(srl.windowSec)
		return true
	}


	// If the cache window is still going on BUT more than max requests..
	if shard.counts[clientIP] >= srl.maxCount {
		return false // Blocked!
	}

	shard.counts[clientIP]++
	return true
}

// Middleware wraps the handler with ultra-fast sharded rate limiting
func (srl *SharedRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}

		if !srl.Allow(host) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}` + "\n"))
			return
		}

		next.ServeHTTP(w, r)
	})
}


