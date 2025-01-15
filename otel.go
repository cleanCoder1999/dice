package main

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTraceProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

// newPropagator creates an instance of [propagation.TextMapPropagator]
//
// Cross-cutting concerns send their state to the next process using Propagators,
// which are defined as objects used to read and write context data to and from
// messages exchanged by the applications.
// Each concern creates a set of Propagators for every supported Propagator type.
//
// NOTE:
// Propagators leverage the Context to inject and extract data
// for each cross-cutting concern, such as traces and Baggage.
//
// Propagation is usually implemented via a cooperation of
// library-specific request interceptors and Propagators,
// where the interceptors detect incoming and outgoing requests
// and use the Propagator’s extract and inject operations respectively.
//
// NOTE:
// The Propagators API is expected to be leveraged by users writing instrumentation libraries.
func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// newTraceProvider creates an instance of [trace.TracerProvider]
//
// To create spans, you need to acquire or initialize a tracer first.
//
// To do that, initialize an exporter, resources, tracer provider, and finally a tracer
func newTraceProvider() (*trace.TracerProvider, error) {
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter,
			// Default is 5s. Set to 1s for demonstrative purposes.
			trace.WithBatchTimeout(time.Second)),
	)
	return traceProvider, nil
}

// newMeterProvider creates an instance of [metric.MeterProvider]
//
// To start producing metrics, you’ll need to have an initialized [metric.MeterProvider]
// that lets you create a [metric.Meter]
//
// Meters let you create instruments that you can use to create different kinds of metrics.
//
// OpenTelemetry Go currently supports the following instruments:
//
//	(1) Counter, a synchronous instrument that supports non-negative increments
//	(2) Asynchronous Counter, an asynchronous instrument which supports non-negative increments
//	(3) Histogram, a synchronous instrument that supports arbitrary values that are statistically meaningful, such as histograms, summaries, or percentile
//	(4) Synchronous Gauge, a synchronous instrument that supports non-additive values, such as room temperature
//	(5) Asynchronous Gauge, an asynchronous instrument that supports non-additive values, such as room temperature
//	(6) UpDownCounter, a synchronous instrument that supports increments and decrements, such as the number of active requests
//	(7) Asynchronous UpDownCounter, an asynchronous instrument that supports increments and decrements
//
// NOTE:
// If a [metric.MeterProvider] is not created either by an instrumentation library or manually,
// the OpenTelemetry Metrics API will use a no-op implementation and fail to generate data.
//
// Here you can find more detailed package documentation for:
//
//	(1) Metrics API: https://go.opentelemetry.io/otel/metric
//	(2) Metrics SDK: https://go.opentelemetry.io/otel/sdk/metric
func newMeterProvider() (*metric.MeterProvider, error) {
	// a metric exporter must be created which in turn will be passed into a meter provider instance
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		return nil, err
	}

	// the meterprovider is configured with a resource that can be customized via the options param
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			// Default is 1m. Set to 3s for demonstrative purposes.
			//
			// NOTE: this interval defines how often the configured resource is printed
			//  => the resource can be configured via the options param of NewMeterProvider(options ...Option)
			metric.WithInterval(10*time.Second))),
	)
	return meterProvider, nil
}

// newLoggerProvider creates an instance of [log.LoggerProvider]
//
// Logs are distinct from metrics and traces in that there is no user-facing OpenTelemetry logs API.
//
// Instead, there is tooling to bridge logs from existing popular log packages
// (such as slog, logrus, zap, logr) into the OpenTelemetry ecosystem.
//
// There are two typical workflows:
// (1) [direct-to-collector]
// (2) [via file or stdout]
//
// [direct-to-collector]: https://opentelemetry.io/docs/languages/go/instrumentation/#direct-to-collector
// [via file or stdout]: https://opentelemetry.io/docs/languages/go/instrumentation/#via-file-or-stdout
func newLoggerProvider() (*log.LoggerProvider, error) {
	logExporter, err := stdoutlog.New()
	if err != nil {
		return nil, err
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	)
	return loggerProvider, nil
}
