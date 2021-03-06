package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/johananl/otel-demo/pkg/role/tracing"
	pb "github.com/johananl/otel-demo/proto/role"
	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/key"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/exporter/trace/jaeger"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
)

var roles []string = []string{
	"coordinator",
	"manager",
	"trainer",
	"dictator",
	"tamer",
	"analyst",
	"engineer",
	"evangelist",
	"designer",
	"plumber",
	"consultant",
	"optimizer",
	"specialist",
}

type server struct {
	pb.UnimplementedRoleServer
}

func (s *server) GetRole(ctx context.Context, in *pb.RoleRequest) (*pb.RoleReply, error) {
	log.Println("Received role request")

	if in.Slow {
		time.Sleep(time.Duration(rand.Intn(300)) * time.Millisecond)
	}
	selected := roles[rand.Intn(len(roles))]

	// Get current span. The span was created within the gRPC interceptor.
	// We are just adding data to it here.
	span := trace.SpanFromContext(ctx)
	span.AddEvent(ctx, "Selected role", key.New("role").String(selected))

	return &pb.RoleReply{Role: selected}, nil
}

func initTraceProvider() {
	// Create a Jaeger exporter.
	exporter, err := jaeger.NewExporter(
		jaeger.WithCollectorEndpoint("http://localhost:14268/api/traces"),
		jaeger.WithProcess(jaeger.Process{
			ServiceName: "role",
			Tags: []core.KeyValue{
				key.String("exporter", "jaeger"),
			},
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create a trace provider.
	// The provider creates a tracer and plugs in the exporter to it.
	tp, err := sdktrace.NewProvider(
		sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithSyncer(exporter),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Register the trace provider.
	global.SetTraceProvider(tp)
}

func main() {
	initTraceProvider()

	rand.Seed(time.Now().UTC().UnixNano())

	host := "localhost"
	port := 9092
	addr := fmt.Sprintf("%s:%d", host, port)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("cannot listen: %v", err)
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(tracing.UnaryServerInterceptor))
	pb.RegisterRoleServer(s, &server{})

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
