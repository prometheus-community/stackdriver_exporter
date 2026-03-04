package otelcollector

import (
	"log/slog"

	prombridge "github.com/ArthurSens/prometheus-collector-bridge"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/receiver"
)

var receiverType = component.MustNewType("stackdriver_exporter")

func NewFactory() receiver.Factory {
	return prombridge.NewFactory(
		receiverType,
		newLifecycleManager(slog.Default()),
		configUnmarshaler{},
		prombridge.WithComponentDefaults(defaultComponentDefaults()),
	)
}

// Keep compiler checks close to factory wiring.
var (
	_ prombridge.ExporterLifecycleManager = (*lifecycleManager)(nil)
	_ prombridge.ConfigUnmarshaler        = (configUnmarshaler{})
)

