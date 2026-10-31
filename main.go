package main

import (
	"log"
	"net/http"
)


func main(){
	tracer,err:= NewTracer("127.0.0.1:50051")
	if err != nil {
		log.Fatalf("failed to init tracer: %v", err)
	}
	defer tracer.Close()


	// 2. Connect to ByteCache on :6379 (The Bouncer's Notebook)
	// Max 100 requests per 60-second window

	limiter := NewRateLimiter("127.0.0.1:6379", 100, 60)


	// 3. Point the Reverse Proxy to GoBalance on :8000 (The Kitchen/Backends)
	proxyHandler,err:= NewReverseProxy("http://127.0.0.1:8000")

	if err != nil {
		log.Fatalf("failed to init reverse proxy: %v", err)
	}

	handler:= tracer.Middleware(limiter.Middleware(proxyHandler))

	// 5. Start listening for incoming traffic on port :8080!
	log.Println("🚀 PortPulse API Gateway listening on :8080...")
	log.Println("   ├── Tracing   -> TracePulse (:50051)")
	log.Println("   ├── Limiter   -> ByteCache  (:6379)")
	log.Println("   └── Proxy     -> GoBalance  (:8000)")

	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}