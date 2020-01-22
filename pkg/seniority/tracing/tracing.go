package tracing

import (
	"context"

	"go.opentelemetry.io/otel/api/distributedcontext"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/plugin/grpctrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var tr = global.TraceProvider().Tracer("seniority")

// UnaryServerInterceptor intercepts and extracts incoming trace data.
func UnaryServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	requestMetadata, _ := metadata.FromIncomingContext(ctx)
	metadataCopy := requestMetadata.Copy()

	entries, spanCtx := grpctrace.Extract(ctx, &metadataCopy)
	ctx = distributedcontext.WithMap(ctx, distributedcontext.NewMap(distributedcontext.MapUpdate{
		MultiKV: entries,
	}))

	ctx, span := tr.Start(
		ctx,
		"handle-grpc-request",
		trace.ChildOf(spanCtx),
		trace.WithSpanKind(trace.SpanKindServer),
	)
	defer span.End()

	return handler(ctx, req)
}

func setTraceStatus(ctx context.Context, err error) {
	if err != nil {
		s, _ := status.FromError(err)
		trace.SpanFromContext(ctx).AddEvent(ctx, err.Error())
		trace.SpanFromContext(ctx).SetStatus(s.Code())
	} else {
		trace.SpanFromContext(ctx).SetStatus(codes.OK)
	}
}
