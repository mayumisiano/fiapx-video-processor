// Package metrics exposes Prometheus instrumentation shared across the
// three binaries: HTTP metrics for identity-api/video-api, and processing
// metrics for video-worker (which has no HTTP router of its own).
package metrics

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests processed, by service, method, route and status.",
	}, []string{"service", "method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds",
		Help: "HTTP request duration in seconds, by service, method and route.",
	}, []string{"service", "method", "path"})

	VideosProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "videos_processed_total",
		Help: "Total video processing outcomes, by status: completed|failed (business outcome, recorded on the request) or infra_error (message nacked to the dead-letter queue).",
	}, []string{"status"})

	ProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "video_processing_duration_seconds",
		Help:    "Duration of a single video processing run, from dequeue to outcome.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s .. ~34min
	})
)

// GinMiddleware records request count and latency for every route handled
// by the given service's Gin router. Uses c.FullPath() (the matched route
// pattern, e.g. "/videos/:id/download") rather than the raw URL, so path
// parameters don't blow up label cardinality; unmatched routes (404s)
// collapse into a single "unmatched" label.
func GinMiddleware(service string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}

		httpRequestsTotal.WithLabelValues(service, c.Request.Method, path, strconv.Itoa(c.Writer.Status())).Inc()
		httpRequestDuration.WithLabelValues(service, c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}

// Handler exposes the registered metrics in the Prometheus text format, for
// mounting under a service's own router (e.g. router.GET("/metrics", ...)).
func Handler() http.Handler {
	return promhttp.Handler()
}

// ServeStandalone starts a bare HTTP server exposing only /metrics, for
// video-worker, which otherwise has no HTTP server. Runs until the process
// exits; failures are logged, not fatal, since metrics are observability,
// not a correctness dependency for the worker's actual job.
func ServeStandalone(addr string) {
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("metrics server on %s stopped: %v", addr, err)
		}
	}()
}
