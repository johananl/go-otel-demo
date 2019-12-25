package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/johananl/otel-demo/pkg/field"
	"github.com/johananl/otel-demo/pkg/seniority"
	fieldpb "github.com/johananl/otel-demo/proto/field"
	senioritypb "github.com/johananl/otel-demo/proto/seniority"
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

	fieldHost := "localhost"
	fieldPort := 9091

	sConn, err := grpc.Dial(
		fmt.Sprintf("%s:%d", seniorityHost, seniorityPort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(seniority.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to seniority service: %v", err)
	}
	defer sConn.Close()
	seniority := senioritypb.NewSeniorityClient(sConn)
	log.Printf("Connected to seniority service at %s:%d\n", seniorityHost, seniorityPort)

	fConn, err := grpc.Dial(
		fmt.Sprintf("%s:%d", fieldHost, fieldPort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(field.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to field service: %v", err)
	}
	defer fConn.Close()
	field := fieldpb.NewFieldClient(fConn)
	log.Printf("Connected to field service at %s:%d\n", fieldHost, fieldPort)

	fakeTitleHandler := func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tr.Start(r.Context(), "serve-http-request")
		defer span.End()

		sr, err := seniority.GetSeniority(ctx, &senioritypb.SeniorityRequest{})
		if err != nil {
			log.Printf("seniority request: %v", err)
			http.Error(w, "Error getting seniority", 500)
		}

		fr, err := field.GetField(ctx, &fieldpb.FieldRequest{})
		if err != nil {
			log.Printf("field request: %v", err)
			http.Error(w, "Error getting field", 500)
		}

		w.Write([]byte(fmt.Sprintf("%s %s", sr.Seniority, fr.Field)))
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
