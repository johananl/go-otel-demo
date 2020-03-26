package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
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

func initTraceProvider(jaegerHost, jaegerPort string) {
	// Create a Jaeger exporter.
	exporter, err := jaeger.NewExporter(
		jaeger.WithCollectorEndpoint(fmt.Sprintf("http://%s:%s/api/traces", jaegerHost, jaegerPort)),
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

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func main() {
	host := getenv("FRONTEND_HOST", "localhost")
	port, err := strconv.Atoi(getenv("FRONTEND_PORT", "8080"))
	if err != nil {
		log.Fatalf("Invalid port %q", port)
	}

	jaegerHost := getenv("FRONTEND_JAEGER_HOST", "localhost")
	jaegerPort := getenv("FRONTEND_JAEGER_PORT", "14268")

	seniorityHost := getenv("FRONTEND_SENIORITY_HOST", "localhost")
	seniorityPort := getenv("FRONTEND_SENIORITY_PORT", "9090")
	fieldHost := getenv("FRONTEND_FIELD_HOST", "localhost")
	fieldPort := getenv("FRONTEND_FIELD_PORT", "9091")
	roleHost := getenv("FRONTEND_ROLE_HOST", "localhost")
	rolePort := getenv("FRONTEND_ROLE_PORT", "9092")

	initTraceProvider(jaegerHost, jaegerPort)
	tr := global.TraceProvider().Tracer("frontend")

	// Connect to seniority service.
	sConn, err := grpc.Dial(
		fmt.Sprintf("%s:%s", seniorityHost, seniorityPort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to seniority service: %v", err)
	}
	defer sConn.Close()
	seniorityClient := senioritypb.NewSeniorityClient(sConn)
	log.Printf("Connected to seniority service at %s:%s\n", seniorityHost, seniorityPort)

	// Connect to field service.
	fConn, err := grpc.Dial(
		fmt.Sprintf("%s:%s", fieldHost, fieldPort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to field service: %v", err)
	}
	defer fConn.Close()
	fieldClient := fieldpb.NewFieldClient(fConn)
	log.Printf("Connected to field service at %s:%s\n", fieldHost, fieldPort)

	// Connect to role service.
	rConn, err := grpc.Dial(
		fmt.Sprintf("%s:%s", roleHost, rolePort),
		grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second),
		grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor),
	)
	if err != nil {
		log.Fatalf("connecting to role service: %v", err)
	}
	defer rConn.Close()
	roleClient := rolepb.NewRoleClient(rConn)
	log.Printf("Connected to role service at %s:%s\n", roleHost, rolePort)

	// API handler function.
	apiHandler := func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tr.Start(r.Context(), "serve-http-request")
		defer span.End()

		var seniority string
		var field string
		var role string
		var res Response

		slow := r.URL.Query().Get("slow")
		if slow != "" {
			// Handle request slowly.

			// Get seniority.
			sr, err := seniorityClient.GetSeniority(ctx, &senioritypb.SeniorityRequest{Slow: true})
			if err != nil {
				log.Printf("getting seniority: %v", err)
				http.Error(w, "Error from seniority service", 500)
				return
			}
			seniority = sr.Seniority

			// Get field.
			fr, err := fieldClient.GetField(ctx, &fieldpb.FieldRequest{Slow: true})
			if err != nil {
				log.Printf("getting field: %v", err)
				http.Error(w, "Error from field service", 500)
				return
			}
			field = fr.Field

			// Get role.
			rr, err := roleClient.GetRole(ctx, &rolepb.RoleRequest{Slow: true})
			if err != nil {
				log.Printf("getting field: %v", err)
				http.Error(w, "Error from field service", 500)
				return
			}
			role = rr.Role

			res = Response{
				Seniority: seniority,
				Field:     field,
				Role:      role,
			}
		} else {
			// Handle request quickly.

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

			res = Response{
				Seniority: seniority,
				Field:     field,
				Role:      role,
			}
		}

		j, err := json.Marshal(res)
		if err != nil {
			log.Println("Error serializing to JSON")
			http.Error(w, "Error serializing to JSON", 500)
			return
		}

		span.AddEvent(ctx, "Generating response", key.New("response").String(string(j)))

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
