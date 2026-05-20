package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := LoadConfig()
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	res, err := newResource()
	if err != nil {
		log.Error("failed to create resource", "error", err)
		os.Exit(1)
	}

	meterProvider, err := newMeterProvider(ctx, cfg, res)
	if err != nil {
		log.Error("failed to create meter provider", "error", err)
		os.Exit(1)
	}

	otel.SetMeterProvider(meterProvider)

	defer func() {
		// Use a fresh context so shutdown can flush even after cancellation.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			log.Error("shutdown", "err", err)
		}
	}()

	client := NewPureGymClient(Credentials{
		email: cfg.Email,
		pin:   cfg.PIN,
	})

	collector, err := NewCollector(client, cfg.GymId, cfg.GymName, cfg.PollInterval, log)
	if err != nil {
		log.Error("create collector", "err", err)
		os.Exit(1)
	}

	log.Info("starting",
		"gym_id", cfg.GymId,
		"gym_name", cfg.GymName,
		"interval", cfg.PollInterval,
		"otlp_endpoint", cfg.OTLPEndpoint,
	)
	collector.Run(ctx)
	log.Info("stopped")
}

func newResource() (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("puregym-collector"),
			semconv.ServiceVersion("0.1.0"),
		),
	)
}

func newMeterProvider(ctx context.Context, cfg *Config, res *resource.Resource) (*metric.MeterProvider, error) {
	exporter, err := otlpmetricgrpc.New(
		ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(exporter,
			metric.WithInterval(60*time.Second))),
	)
	return meterProvider, nil
}
