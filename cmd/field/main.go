package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/johananl/otel-demo/pkg/field/tracing"
	pb "github.com/johananl/otel-demo/proto/field"
	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/key"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/exporter/trace/jaeger"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
)

var fields []string = []string{
	"marketing",
	"dolphin",
	"cat",
	"penguin",
	"engineering",
	"aerospace",
	"machinery",
	"finance",
	"strategy",
	"beer",
	"coffee",
	"whisky",
	"laundry",
	"socks",
}

type server struct {
	pb.UnimplementedFieldServer
}

func (s *server) GetField(ctx context.Context, in *pb.FieldRequest) (*pb.FieldReply, error) {
	log.Println("Received field request")

	if in.Slow {
		time.Sleep(time.Duration(rand.Intn(300)) * time.Millisecond)
	}
	selected := fields[rand.Intn(len(fields))]

	// Get current span. The span was created within the gRPC interceptor.
	// We are just adding data to it here.
	span := trace.SpanFromContext(ctx)
	span.AddEvent(ctx, "Selected field", key.New("field").String(selected))

	return &pb.FieldReply{Field: selected}, nil
}

func initTraceProvider() {
	// Create a Jaeger exporter.
	exporter, err := jaeger.NewExporter(
		jaeger.WithCollectorEndpoint("http://localhost:14268/api/traces"),
		jaeger.WithProcess(jaeger.Process{
			ServiceName: "field",
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
	port := 9091
	addr := fmt.Sprintf("%s:%d", host, port)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("cannot listen: %v", err)
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(tracing.UnaryServerInterceptor))
	pb.RegisterFieldServer(s, &server{})

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
