package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/johananl/otel-demo/pkg/seniority/tracing"
	pb "github.com/johananl/otel-demo/proto/seniority"
	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/key"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/exporter/trace/jaeger"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
)

var seniorities []string = []string{
	"senior",
	"junior",
	"assistant",
	"executive",
	"intergalactic",
}

type server struct {
	pb.UnimplementedSeniorityServer
}

func (s *server) GetSeniority(ctx context.Context, in *pb.SeniorityRequest) (*pb.SeniorityReply, error) {
	log.Println("Received seniority request")

	selected := seniorities[rand.Intn(len(seniorities))]

	// Get current span. The span was created within the gRPC interceptor.
	// We are just adding data to it here.
	span := trace.SpanFromContext(ctx)
	span.AddEvent(ctx, "Selected seniority", key.New("seniority").String(selected))

	return &pb.SeniorityReply{Seniority: selected}, nil
}

func initTracer() {
	exporter, err := jaeger.NewExporter(
		jaeger.WithCollectorEndpoint("http://localhost:14268/api/traces"),
		jaeger.WithProcess(jaeger.Process{
			ServiceName: "seniority",
			Tags: []core.KeyValue{
				key.String("exporter", "jaeger"),
			},
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	tp, err := sdktrace.NewProvider(
		sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithSyncer(exporter),
	)
	if err != nil {
		log.Fatal(err)
	}

	global.SetTraceProvider(tp)
}

func main() {
	initTracer()

	rand.Seed(time.Now().UTC().UnixNano())

	host := "localhost"
	port := 9090
	addr := fmt.Sprintf("%s:%d", host, port)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("cannot listen: %v", err)
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(tracing.UnaryServerInterceptor))
	pb.RegisterSeniorityServer(s, &server{})

	ch := make(chan struct{})
	go func(ch chan struct{}) {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		ch <- struct{}{}
	}(ch)
	log.Printf("Listening for gRPC connections on port %d", port)

	<-ch
}
