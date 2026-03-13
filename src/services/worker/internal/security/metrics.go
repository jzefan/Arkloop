package security

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ScanTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "arkloop",
		Subsystem: "injection",
		Name:      "scan_total",
		Help:      "Total number of injection scans performed.",
	}, []string{"result"})

	DetectionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "arkloop",
		Subsystem: "injection",
		Name:      "detection_total",
		Help:      "Total injection detections by category.",
	}, []string{"category"})
)
