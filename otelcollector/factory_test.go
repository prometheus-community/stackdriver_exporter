package otelcollector

import (
	"context"
	"testing"

	prombridge "github.com/ArthurSens/prometheus-collector-bridge"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestNewFactory(t *testing.T) {
	t.Parallel()

	factory := NewFactory()
	if factory == nil {
		t.Fatal("NewFactory() returned nil")
	}
}

func TestFactory_CreateMetrics(t *testing.T) {
	t.Parallel()

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*prombridge.ReceiverConfig)

	cfg.ExporterConfig = map[string]interface{}{
		"project_ids":      []string{"my-project"},
		"metrics_prefixes": []string{"compute.googleapis.com/instance"},
	}

	settings := receivertest.NewNopSettings(receiverType)
	consumer := new(consumertest.MetricsSink)

	recv, err := factory.CreateMetrics(context.Background(), settings, cfg, consumer)
	if err != nil {
		t.Fatalf("CreateMetrics() error = %v", err)
	}
	if recv == nil {
		t.Fatal("CreateMetrics() returned nil receiver")
	}
}

