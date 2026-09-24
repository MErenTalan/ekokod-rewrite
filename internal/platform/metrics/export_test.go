package metrics_test

import "github.com/prometheus/client_golang/prometheus"

func prometheusGatherer() prometheus.Gatherer { return prometheus.DefaultGatherer }
