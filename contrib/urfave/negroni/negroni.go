package negroni

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
)

type DatadogMiddleware struct {
	cfg *config
}

type StatusRecorder struct {
	http.ResponseWriter
	Status int
}

func (r *StatusRecorder) WriteHeader(status int) {
	r.Status = status
	r.ResponseWriter.WriteHeader(status)
}

func (m *DatadogMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	opts := []ddtrace.StartSpanOption{
		tracer.SpanType(ext.SpanTypeWeb),
		tracer.ServiceName(m.cfg.serviceName),
		tracer.Tag(ext.HTTPMethod, r.Method),
		tracer.Tag(ext.HTTPURL, r.URL.Path),
		tracer.Measured(),
	}
	if !math.IsNaN(m.cfg.analyticsRate) {
		opts = append(opts, tracer.Tag(ext.EventSampleRate, m.cfg.analyticsRate))
	}
	if spanctx, err := tracer.Extract(tracer.HTTPHeadersCarrier(r.Header)); err == nil {
		opts = append(opts, tracer.ChildOf(spanctx))
	}
	opts = append(opts, m.cfg.spanOpts...)
	span, ctx := tracer.StartSpanFromContext(r.Context(), "http.request", opts...)
	defer span.Finish()

	r = r.WithContext(ctx)
	recorder := &StatusRecorder{
		ResponseWriter: w,
		Status:         200,
	}

	next(recorder, r)

	span.SetTag(ext.HTTPCode, strconv.Itoa(recorder.Status))
	if recorder.Status >= 500 && recorder.Status < 600 {
		span.SetTag(ext.Error, fmt.Errorf("%d: %s", recorder.Status, http.StatusText(recorder.Status)))
	}
}

func Middleware(opts ...Option) *DatadogMiddleware {
	cfg := new(config)
	defaults(cfg)
	for _, fn := range opts {
		fn(cfg)
	}
	log.Debug("contrib/urgave/negroni: Configuring Middleware: %#v", cfg)

	m := DatadogMiddleware{
		cfg: cfg,
	}

	return &m
}
