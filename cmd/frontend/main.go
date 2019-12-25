package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/johananl/otel-demo/pkg/field"
	"github.com/johananl/otel-demo/pkg/role"
	"github.com/johananl/otel-demo/pkg/seniority"
	fieldpb "github.com/johananl/otel-demo/proto/field"
	rolepb "github.com/johananl/otel-demo/proto/role"
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

	seniorityHost := "localhost"
	seniorityPort := 9090
	fieldHost := "localhost"
	fieldPort := 9091
	roleHost := "localhost"
	rolePort := 9092

	sConn, err := grpc.Dial(
		fmt.Sprintf("%s:%d", seniorityHost, seniorityPort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(seniority.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to seniority service: %v", err)
	}
	defer sConn.Close()
	seniorityClient := senioritypb.NewSeniorityClient(sConn)
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
	fieldClient := fieldpb.NewFieldClient(fConn)
	log.Printf("Connected to field service at %s:%d\n", fieldHost, fieldPort)

	rConn, err := grpc.Dial(
		fmt.Sprintf("%s:%d", roleHost, rolePort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(role.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to role service: %v", err)
	}
	defer rConn.Close()
	roleClient := rolepb.NewRoleClient(rConn)
	log.Printf("Connected to role service at %s:%d\n", roleHost, rolePort)

	fakeTitleHandler := func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tr.Start(r.Context(), "serve-http-request")
		defer span.End()

		var seniority string
		var field string
		var role string

		errChan := make(chan error)

		// Get seniority.
		sChan := make(chan *senioritypb.SeniorityReply)
		go func(reply chan<- *senioritypb.SeniorityReply, errChan chan<- error) {
			defer close(reply)

			r, err := seniorityClient.GetSeniority(ctx, &senioritypb.SeniorityRequest{})
			if err != nil {
				errChan <- fmt.Errorf("getting seniority: %v", err)
				return
			}

			reply <- r
		}(sChan, errChan)

		// Get field.
		fChan := make(chan *fieldpb.FieldReply)
		go func(reply chan<- *fieldpb.FieldReply, errChan chan<- error) {
			defer close(reply)

			r, err := fieldClient.GetField(ctx, &fieldpb.FieldRequest{})
			if err != nil {
				errChan <- fmt.Errorf("getting field: %v", err)
				return
			}

			reply <- r
		}(fChan, errChan)

		// Get role.
		rChan := make(chan *rolepb.RoleReply)
		go func(reply chan<- *rolepb.RoleReply, errChan chan<- error) {
			defer close(reply)

			r, err := roleClient.GetRole(ctx, &rolepb.RoleRequest{})
			if err != nil {
				errChan <- fmt.Errorf("getting role: %v", err)
				return
			}

			reply <- r
		}(rChan, errChan)

		select {
		case sr := <-sChan:
			seniority = sr.Seniority
		case fr := <-fChan:
			field = fr.Field
		case rr := <-rChan:
			role = rr.Role
		case err := <-errChan:
			log.Printf("gRPC error: %v", err)
			http.Error(w, "Error from backend service", 500)
			return
		}

		// Write HTTP response.
		w.Write([]byte(fmt.Sprintf("%s %s %s", seniority, field, role)))
	}

	http.HandleFunc("/", fakeTitleHandler)

	addr := fmt.Sprintf("%s:%d", host, port)
	ch := make(chan struct{})
	go func(ch chan struct{}) {
		log.Fatal(http.ListenAndServe(addr, nil))
		ch <- struct{}{}
	}(ch)
	log.Printf("Listening for HTTP requests on port %d", port)

	<-ch
}
