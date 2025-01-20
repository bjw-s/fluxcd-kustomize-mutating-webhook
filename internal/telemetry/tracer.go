package telemetry

import (
	"context"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func InitTracer() func() {
	return initTracerWithProvider(initRealTracerProvider)
}

func initTracerWithProvider(providerInitializer func() (trace.TracerProvider, error)) func() {
	provider, err := providerInitializer()
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize tracer provider")
		return func() {}
	}

	otel.SetTracerProvider(provider)

	return func() {
		if sdkProvider, ok := provider.(*sdktrace.TracerProvider); ok {
			if err := sdkProvider.Shutdown(context.Background()); err != nil {
				log.Error().Err(err).Msg("Failed to shutdown tracer provider")
			}
		}
	}
}

func initRealTracerProvider() (trace.TracerProvider, error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("kustomize-mutating-webhook"),
			attribute.String("environment", "production"),
		),
	)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient("otel-collector:4317", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(otlptracegrpc.WithGRPCConn(conn)))
	if err != nil {
		return nil, err
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	return tracerProvider, nil
}
