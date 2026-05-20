package main

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Collector struct {
	client   *PureGymClient
	interval time.Duration
	log      *slog.Logger

	attrs     metric.MeasurementOption
	occupancy metric.Int64Gauge
	scrapeOK  metric.Int64Gauge

	gymId int
}

func NewCollector(client *PureGymClient, gymId int, gymName string, interval time.Duration, log *slog.Logger) (*Collector, error) {
	meter := otel.Meter("puregym-collector")

	occupancy, err := meter.Int64Gauge("puregym.gym.occupancy",
		metric.WithDescription("People currently in the gym"),
		metric.WithUnit("{people}"),
	)
	if err != nil {
		return nil, err
	}

	scrapeOK, err := meter.Int64Gauge("puregym.scrape.success",
		metric.WithDescription("1 if the most recent scrape succeeded, 0 otherwise"),
	)
	if err != nil {
		return nil, err
	}

	return &Collector{
		client:   client,
		gymId:    gymId,
		interval: interval,
		log:      log,
		attrs: metric.WithAttributes(
			attribute.Int("gym_id", gymId),
			attribute.String("gym_name", gymName),
		),
		occupancy: occupancy,
		scrapeOK:  scrapeOK,
	}, nil
}

func (c *Collector) Run(ctx context.Context) {
	c.poll(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.poll(ctx)
		}
	}
}

func (c *Collector) poll(ctx context.Context) {
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	count, err := c.client.FetchOccupancy(reqCtx, c.gymId)
	if err != nil {
		c.log.Warn("scrape failed", "err", err)
		c.scrapeOK.Record(ctx, 0, c.attrs)
		return
	}

	c.log.Info("scraped", "occupancy", count)

	c.occupancy.Record(ctx, int64(count), c.attrs)
	c.scrapeOK.Record(ctx, 1, c.attrs)
}
