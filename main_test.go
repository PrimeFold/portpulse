package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	pb "portpulse/proto"

	"google.golang.org/grpc"
)

// mockTraceServer captures incoming spans in memory for verification
type mockTraceServer struct {
	pb.UnimplementedTelemetryCollectorServer
	spanCount int64
}

func (m *mockTraceServer) IngestSpans(ctx context.Context, req *pb.IngestReqest) (*pb.IngestResponse, error) {
	atomic.AddInt64(&m.spanCount, int64(len(req.Spans)))
	return &pb.IngestResponse{Success: true, ReceivedCount: int32(len(req.Spans))}, nil
}

// mockByteCacheServer simulates ByteCache over raw TCP
func startMockByteCache(t *testing.T) (string, func()) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on bytecache mock: %v", err)
	}

	counts := make(map[string]int)

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var cmd, key string
				var val, ttl int
				fmt.Fscanf(c, "%s %s", &cmd, &key)
				switch cmd {
				case "GET":
					if countVal, ok := counts[key]; ok {
						fmt.Fprintf(c, "VALUE %d\n", countVal)
					} else {
						fmt.Fprintf(c, "ERR key not found\n")
					}
				case "SET":
					fmt.Fscanf(c, "%d %d", &val, &ttl)
					counts[key] = val
					fmt.Fprintf(c, "OK\n")
				}
			}(conn)
		}
	}()

	return lis.Addr().String(), func() { lis.Close() }
}

func TestPortPulseEndToEnd(t *testing.T) {
	// 1. Start mock backend (representing GoBalance target)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	// 2. Start mock TracePulse gRPC server
	traceLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("trace listen error: %v", err)
	}
	defer traceLis.Close()

	mockTrace := &mockTraceServer{}
	grpcServer := grpc.NewServer()
	pb.RegisterTelemetryCollectorServer(grpcServer, mockTrace)
	go func() { _ = grpcServer.Serve(traceLis) }()
	defer grpcServer.Stop()

	// 3. Start mock ByteCache server
	cacheAddr, closeCache := startMockByteCache(t)
	defer closeCache()

	// 4. Initialize PortPulse components
	tracer, err := NewTracer(traceLis.Addr().String())
	if err != nil {
		t.Fatalf("init tracer error: %v", err)
	}
	defer tracer.Close()

	// Limit to 5 requests
	limiter := NewRateLimiter(cacheAddr, 5, 60)

	proxyHandler, err := NewReverseProxy(backend.URL)
	if err != nil {
		t.Fatalf("init proxy error: %v", err)
	}

	handler := tracer.Middleware(limiter.Middleware(proxyHandler))

	// 5. Test normal requests (1 to 5 should succeed with 200 OK)
	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 OK, got %d", i, rec.Code)
		}
	}

	// 6. Test rate-limited request (6th request MUST be blocked with 429)
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rec.Code)
	}

	// 7. Verify TracePulse received telemetry spans
	time.Sleep(50 * time.Millisecond) // Give async span shippers a moment
	if count := atomic.LoadInt64(&mockTrace.spanCount); count != 6 {
		t.Fatalf("expected at least 5 spans recorded in TracePulse, got %d", count)
	}
}

// BenchmarkPortPulse measures the throughput of the full middleware stack
func BenchmarkPortPulse(b *testing.B) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxyHandler, _ := NewReverseProxy(backend.URL)

	req := httptest.NewRequest("GET", "/bench", nil)
	rec := httptest.NewRecorder()

	
	b.ReportAllocs()

	for b.Loop() {
		proxyHandler.ServeHTTP(rec, req)
	}
}


// BenchmarkShardedRateLimiter measures the pure throughput of 32 parallel shards
func BenchmarkShardedRateLimiter(b *testing.B) {
	limiter := NewSharedRateLimiter(1000000, 60)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow("192.168.1.100")
		}
	})
}