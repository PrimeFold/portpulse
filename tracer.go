package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	pb "portpulse/proto"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)



type Tracer struct{
	conn *grpc.ClientConn
	client pb.TelemetryCollectorClient
}


type statusRecorder struct{
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(code int){
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func NewTracer(target string) (*Tracer , error){
	conn,err:= grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil{
		return nil, fmt.Errorf("failed to connect to TracePulse: %w", err)
	}

	client:= pb.NewTelemetryCollectorClient(conn)
	return &Tracer{
		conn:conn,
		client:client,
	},nil

}


func (t *Tracer) Middleware(next http.Handler) http.Handler{
	return http.HandlerFunc(func(w http.ResponseWriter , r *http.Request){
		start:= time.Now()

		rec:= &statusRecorder{
			ResponseWriter: w,
			statusCode: http.StatusOK,
		}

		next.ServeHTTP(rec,r)

		duration:= time.Since(start).Milliseconds()

		span:= &pb.Span{
			TraceId:       fmt.Sprintf("trace-%d", time.Now().UnixNano()),
			SpanId:        fmt.Sprintf("span-%d", time.Now().UnixNano()%1000000),
			ServiceName:   "portpulse-gateway",
			OperationName: fmt.Sprintf("%s %s", r.Method, r.URL.Path),
			StatusCode:    fmt.Sprintf("%d", rec.statusCode),
			DurationMs:    duration,
		}

		go func (s *pb.Span)  {
			ctx,cancel:= context.WithTimeout(context.Background(),2*time.Second)
			defer cancel()
			_,err:= t.client.IngestSpans(ctx,&pb.IngestReqest{Spans: []*pb.Span{s}})

			if err != nil{
				log.Printf("[tracer] failed to ship span to TracePulse: %v", err)
				return
			}
		}(span)

	})
}
func (t *Tracer) Close() error{
	return t.conn.Close()
}




