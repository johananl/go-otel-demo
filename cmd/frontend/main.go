package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/johananl/otel-demo/pkg/seniority"
	pb "github.com/johananl/otel-demo/proto/seniority"
	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/key"
	"go.opentelemetry.io/otel/exporter/trace/jaeger"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
)

func initTracer() {
	exporter, err := jaeger.NewExporter(
		jaeger.WithCollectorEndpoint("http://localhost:14268/api/traces"),
		jaeger.WithProcess(jaeger.Process{
			ServiceName: "frontend",
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
	tr := global.TraceProvider().Tracer("frontend")

	host := "localhost"
	port := 8080
	addr := fmt.Sprintf("%s:%d", host, port)

	seniorityHost := "localhost"
	seniorityPort := 9090

	conn, err := grpc.Dial(
		fmt.Sprintf("%s:%d", seniorityHost, seniorityPort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(seniority.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to seniority service: %v", err)
	}
	defer conn.Close()
	seniority := pb.NewSeniorityClient(conn)

	fakeTitleHandler := func(w http.ResponseWriter, r *http.Request) {
		_, span := tr.Start(r.Context(), "serve-http-request")
		defer span.End()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		sr, err := seniority.GetSeniority(ctx, &pb.SeniorityRequest{})
		if err != nil {
			log.Printf("seniority request: %v", err)
			http.Error(w, "Error getting seniority", 500)
		}

		w.Write([]byte(fmt.Sprintf("%s", sr.Seniority)))
	}

	http.HandleFunc("/", fakeTitleHandler)

	ch := make(chan struct{})
	go func(ch chan struct{}) {
		log.Fatal(http.ListenAndServe(addr, nil))
		ch <- struct{}{}
	}(ch)
	log.Printf("Listening for HTTP requests on port %d", port)

	<-ch
}
