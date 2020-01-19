package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/johananl/otel-demo/pkg/frontend/tracing"
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

type Response struct {
	Seniority string `json:"seniority"`
	Field     string `json:"field"`
	Role      string `json:"role"`
}

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

	// Connect to seniority service.
	sConn, err := grpc.Dial(
		fmt.Sprintf("%s:%d", seniorityHost, seniorityPort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to seniority service: %v", err)
	}
	defer sConn.Close()
	seniorityClient := senioritypb.NewSeniorityClient(sConn)
	log.Printf("Connected to seniority service at %s:%d\n", seniorityHost, seniorityPort)

	// Connect to field service.
	fConn, err := grpc.Dial(
		fmt.Sprintf("%s:%d", fieldHost, fieldPort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to field service: %v", err)
	}
	defer fConn.Close()
	fieldClient := fieldpb.NewFieldClient(fConn)
	log.Printf("Connected to field service at %s:%d\n", fieldHost, fieldPort)

	// Connect to role service.
	rConn, err := grpc.Dial(
		fmt.Sprintf("%s:%d", roleHost, rolePort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to role service: %v", err)
	}
	defer rConn.Close()
	roleClient := rolepb.NewRoleClient(rConn)
	log.Printf("Connected to role service at %s:%d\n", roleHost, rolePort)

	apiHandler := func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tr.Start(r.Context(), "serve-http-request")
		defer span.End()

		var seniority string
		var field string
		var role string

		errChan := make(chan error)

		// Get seniority.
		sChan := make(chan *senioritypb.SeniorityReply)
		go func(reply chan<- *senioritypb.SeniorityReply, errChan chan<- error) {
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
			r, err := roleClient.GetRole(ctx, &rolepb.RoleRequest{})
			if err != nil {
				errChan <- fmt.Errorf("getting role: %v", err)
				return
			}

			reply <- r
		}(rChan, errChan)

		// Wait for all gRPC calls to return.
		for seniority == "" || field == "" || role == "" {
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
		}

		res := Response{
			Seniority: seniority,
			Field:     field,
			Role:      role,
		}

		j, err := json.Marshal(res)
		if err != nil {
			log.Println("Error serializing to JSON")
			http.Error(w, "Error serializing to JSON", 500)
			return
		}

		// Write HTTP response.
		w.Write(j)
	}

	// Handle static content (for UI).
	fs := http.FileServer(http.Dir("ui/build"))
	http.Handle("/", fs)

	// Handle API.
	http.HandleFunc("/api", apiHandler)

	addr := fmt.Sprintf("%s:%d", host, port)
	ch := make(chan struct{})
	go func(ch chan struct{}) {
		log.Fatal(http.ListenAndServe(addr, nil))
		ch <- struct{}{}
	}(ch)
	log.Printf("Listening for HTTP requests on port %d", port)

	<-ch
}
