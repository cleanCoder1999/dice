package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// name is an untyped constant
const name = "go.opentelemetry.io/otel/example/dice"

// the package-level variables use an untyped constant for initialization
var (
	tracer  = otel.Tracer(name)
	meter   = otel.Meter(name)
	logger  = otelslog.NewLogger(name)
	rollCnt metric.Int64Counter
)

func init() {
	var err error
	rollCnt, err = meter.Int64Counter("dice.rolls",
		metric.WithDescription("The number of rolls by roll value"),
		metric.WithUnit("{roll}"),
	)
	if err != nil {
		panic(err)
	}
}

// rolldice generates a random number between 1 and 6, and writes it into the http response
//
// In addition, most of the function body is dedicated to manually instrument its logic by
// (1) logging,
// (2) creating and enriching a span
// (3) and providing metrics
func rolldice(w http.ResponseWriter, r *http.Request) {

	// creates a span that is either a child of r.Context() if it contains a span.
	// Otherwise, it will create a new span that is a root span and a context containing that span
	ctx, span := tracer.Start(r.Context(), "roll")
	defer span.End()

	roll := 1 + rand.Intn(6)

	// the instrumentation code below
	// (1) writes logs via the logger instance
	// (2) sets attributes in the span
	// (3) increments the metric counter
	var msg string
	if player := r.PathValue("player"); player != "" {
		msg = fmt.Sprintf("%s is rolling the dice", player)
	} else {
		msg = "Anonymous player is rolling the dice"
	}
	logger.InfoContext(ctx, msg, "result", roll)

	rollValueAttr := attribute.Int("roll.value", roll)
	span.SetAttributes(rollValueAttr)
	rollCnt.Add(ctx, 1, metric.WithAttributes(rollValueAttr))

	resp := strconv.Itoa(roll) + "\n"
	if _, err := io.WriteString(w, resp); err != nil {
		log.Printf("Write failed: %v\n", err)
	}
}
