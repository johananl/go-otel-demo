package tracing

import (
	"context"

	"go.opentelemetry.io/otel/plugin/grpctrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientInterceptor intercepts and injects outgoing trace data.
func UnaryClientInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	requestMetadata, _ := metadata.FromOutgoingContext(ctx)
	metadataCopy := requestMetadata.Copy()

	grpctrace.Inject(ctx, &metadataCopy)
	ctx = metadata.NewOutgoingContext(ctx, metadataCopy)

	err := invoker(ctx, method, req, reply, cc, opts...)

	return err
}
