package main

import (
	"fmt"
	"log"
	"net/http"

	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/key"
	"go.opentelemetry.io/otel/exporter/trace/jaeger"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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

	fakeTitleHandler := func(w http.ResponseWriter, r *http.Request) {
		_, span := tr.Start(r.Context(), "serve-http-request")
		defer span.End()

		w.Write([]byte("Howdy!"))
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
