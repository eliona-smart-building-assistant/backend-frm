package instrumentation

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultExporterPort = ":5000"
	defaultMetricsPath  = "/runtime/metrics"
)

type prometheusExporter struct {
	s *http.Server
}

// CheckFn is called by when liveness or readiness probes are performed.
// If bool value is True - [http.StatusOK] is returned by handler, otherwise
// [http.StatusInternalServerError] is returned by liveness probe and [http.StatusServiceUnavailable]
// is returned by readiness probe.
//
// If map is provided and probe should return an error status - map will be encoded do JSON and send as body
// in response
type CheckFn func(ctx context.Context) (bool, map[string]string)

var defaultExporter prometheusExporter

// StartExporter launches the webserver on designated port (:5000) which provides:
//
// 1. Go runtime metrics on /runtime/metrics
//
// 2. Liveness probes via liveProbe callback
//
// 3. Readiness probes via readyProbe callback
//
// In liveness probe you likely want to return the errors (if any) like database or kafka not available.
// See description of [CheckFn] for more information.
func StartExporter(liveProbe, readyProbe CheckFn) {
	mux := http.NewServeMux()
	mux.Handle(defaultMetricsPath, promhttp.Handler())
	mux.HandleFunc("/livez", wrapChecker(liveProbe, http.StatusInternalServerError))
	mux.HandleFunc("/readyz", wrapChecker(readyProbe, http.StatusServiceUnavailable))

	server := &http.Server{
		Addr:    defaultExporterPort,
		Handler: mux,
	}

	defaultExporter.s = server

	go func() {
		defaultExporter.s.ListenAndServe()
	}()
}

func StopExporter() {
	if defaultExporter.s == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	defaultExporter.s.Shutdown(ctx)
}

func wrapChecker(fn CheckFn, failCode int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checkOk, checkData := fn(r.Context())
		if !checkOk {
			w.WriteHeader(failCode)
			if checkData != nil {
				json.NewEncoder(w).Encode(checkData)
			}

			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
