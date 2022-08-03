package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

const (

	// start list of metrics
	// Model metrics
	ModelHistogramName                    = "seldon_model_infer_api_seconds"
	ModelInferCounterName                 = "seldon_model_infer_total"
	ModelInferLatencyCounterName          = "seldon_model_infer_seconds_total"
	ModelAggregateInferCounterName        = "seldon_model_aggregate_infer_total"
	ModelAggregateInferLatencyCounterName = "seldon_model_aggregate_infer_seconds_total"
	// Agent metrics
	CacheEvictCounterName                              = "seldon_cache_evict_count"
	CacheMissCounterName                               = "seldon_cache_miss_count"
	LoadModelCounterName                               = "seldon_load_model_counter"
	UnloadModelCounterName                             = "seldon_unload_model_counter"
	LoadedModelGaugeName                               = "seldon_loaded_model_gauge"
	LoadedModelMemoryGaugeName                         = "seldon_loaded_model_memory_bytes_gauge"
	EvictedModelMemoryGaugeName                        = "seldon_evicted_model_memory_bytes_gauge"
	ServerReplicaMemoryCapacityGaugeName               = "seldon_server_replica_memory_capacity_bytes_gauge"
	ServerReplicaMemoryCapacityWithOverCommitGaugeName = "seldon_server_replica_memory_capacity_overcommit_bytes_gauge"
	// end list of metrics

	SeldonModelMetric         = "model"
	SeldonInternalModelMetric = "model_internal"
	SeldonServerMetric        = "server"
	SeldonServerReplicaMetric = "server_replica"
	MethodTypeMetric          = "method_type"
	MethodTypeRest            = "rest"
	MethodTypeGrpc            = "grpc"
	MethodMetric              = "method"
	CodeMetric                = "code"
)

//TODO Revisit splitting this interface as metric handling matures
type AgentMetricsHandler interface {
	AddModelHistogramMetricsHandler(baseHandler http.HandlerFunc) http.HandlerFunc
	UnaryServerInterceptor() func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)
	AddModelInferMetrics(externalModelName string, internalModelName string, method string, elapsedTime float64, code string)
	AddLoadedModelMetrics(internalModelName string, memory uint64, isLoad, isSoft bool)
	AddServerReplicaMetrics(memory uint64, memoryWithOvercommit float32)
}

var (
	DefaultHistogramBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}
)

type PrometheusMetrics struct {
	serverName       string
	serverReplicaIdx string
	namespace        string
	logger           log.FieldLogger
	// Model metrics
	modelHistogram                                 *prometheus.HistogramVec
	modelInferCounter                              *prometheus.CounterVec
	modelInferLatencyCounter                       *prometheus.CounterVec
	modelAggregateInferCounter                     *prometheus.CounterVec
	modelAggregateInferLatencyCounter              *prometheus.CounterVec
	cacheEvictCounter                              *prometheus.CounterVec
	cacheMissCounter                               *prometheus.CounterVec
	loadModelCounter                               *prometheus.CounterVec
	unloadModelCounter                             *prometheus.CounterVec
	loadedModelGauge                               *prometheus.GaugeVec
	loadedModelMemoryGauge                         *prometheus.GaugeVec
	evictedModelMemoryGauge                        *prometheus.GaugeVec
	serverReplicaMemoryCapacityGauge               *prometheus.GaugeVec
	serverReplicaMemoryCapacityWithOvercommitGauge *prometheus.GaugeVec
	server                                         *http.Server
}

func NewPrometheusModelMetrics(serverName string, serverReplicaIdx uint, namespace string, logger log.FieldLogger) (*PrometheusMetrics, error) {
	namespace = safeNamespaceName(namespace)
	histogram, err := createModelHistogram(namespace)
	if err != nil {
		return nil, err
	}

	inferCounter, err := createModelInferCounter(namespace)
	if err != nil {
		return nil, err
	}

	inferLatencyCounter, err := createModelInferLatencyCounter(namespace)
	if err != nil {
		return nil, err
	}

	aggregateInferCounter, err := createModelAggregateInferCounter(namespace)
	if err != nil {

		return nil, err
	}

	aggregateInferLatencyCounter, err := createModelAggregateInferLatencyCounter(namespace)
	if err != nil {
		return nil, err
	}

	cacheEvictCounter, err := createCacheEvictCounter(namespace)
	if err != nil {
		return nil, err
	}

	cacheMissCounter, err := createCacheMissCounter(namespace)
	if err != nil {
		return nil, err
	}

	loadModelCounter, err := createLoadModelCounter(namespace)
	if err != nil {
		return nil, err
	}

	unloadModelCounter, err := createUnloadModelCounter(namespace)
	if err != nil {
		return nil, err
	}

	loadedModelGauge, err := createLoadedModelGauge(namespace)
	if err != nil {
		return nil, err
	}

	loadedModelMemoryGauge, err := createLoadedModelMemoryGauge(namespace)
	if err != nil {
		return nil, err
	}

	evictedModelMemoryGauge, err := createEvictedModelMemoryGauge(namespace)
	if err != nil {
		return nil, err
	}

	serverReplicaMemoryCapacityGauge, err := createServerReplicaMemoryCapacityGauge(namespace)
	if err != nil {
		return nil, err
	}

	serverReplicaMemoryCapacityWithOvercommitGauge, err := createServerReplicaMemoryCapacityWithOvercommitGauge(namespace)
	if err != nil {
		return nil, err
	}

	return &PrometheusMetrics{
		serverName:                        serverName,
		serverReplicaIdx:                  fmt.Sprintf("%d", serverReplicaIdx),
		namespace:                         namespace,
		logger:                            logger.WithField("source", "PrometheusMetrics"),
		modelHistogram:                    histogram,
		modelInferCounter:                 inferCounter,
		modelInferLatencyCounter:          inferLatencyCounter,
		modelAggregateInferCounter:        aggregateInferCounter,
		modelAggregateInferLatencyCounter: aggregateInferLatencyCounter,
		cacheEvictCounter:                 cacheEvictCounter,
		cacheMissCounter:                  cacheMissCounter,
		loadModelCounter:                  loadModelCounter,
		unloadModelCounter:                unloadModelCounter,
		loadedModelGauge:                  loadedModelGauge,
		loadedModelMemoryGauge:            loadedModelMemoryGauge,
		evictedModelMemoryGauge:           evictedModelMemoryGauge,
		serverReplicaMemoryCapacityGauge:  serverReplicaMemoryCapacityGauge,
		serverReplicaMemoryCapacityWithOvercommitGauge: serverReplicaMemoryCapacityWithOvercommitGauge,
	}, nil
}

func safeNamespaceName(namespace string) string {
	return strings.ReplaceAll(namespace, "-", "_")
}

func createModelHistogram(namespace string) (*prometheus.HistogramVec, error) {
	//TODO add method for rest/grpc
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric, MethodMetric, CodeMetric}

	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      ModelHistogramName,
			Namespace: namespace,
			Help:      "A histogram of latencies for inference server",
			Buckets:   DefaultHistogramBuckets,
		},
		labelNames,
	)
	err := prometheus.Register(histogram)
	if err != nil {
		if e, ok := err.(prometheus.AlreadyRegisteredError); ok {
			histogram = e.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			return nil, err
		}
	}
	return histogram, nil
}

func createModelInferCounter(namespace string) (*prometheus.CounterVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric, SeldonModelMetric, SeldonInternalModelMetric, MethodTypeMetric, CodeMetric}
	return createCounterVec(
		ModelInferCounterName, "A count of server inference calls",
		namespace, labelNames)
}

func createModelInferLatencyCounter(namespace string) (*prometheus.CounterVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric, SeldonModelMetric, SeldonInternalModelMetric, MethodTypeMetric, CodeMetric}
	return createCounterVec(
		ModelInferLatencyCounterName, "A sum of server inference call latencies",
		namespace, labelNames)
}

func createModelAggregateInferCounter(namespace string) (*prometheus.CounterVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric, MethodTypeMetric}
	return createCounterVec(
		ModelAggregateInferCounterName, "A count of server inference calls (aggregate)",
		namespace, labelNames)
}

func createModelAggregateInferLatencyCounter(namespace string) (*prometheus.CounterVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric, MethodTypeMetric}
	return createCounterVec(
		ModelAggregateInferLatencyCounterName, "A sum of server inference call latencies (aggregate)",
		namespace, labelNames)
}

func createCacheEvictCounter(namespace string) (*prometheus.CounterVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric}
	return createCounterVec(
		CacheEvictCounterName, "A count of model cache evict",
		namespace, labelNames)
}

func createCacheMissCounter(namespace string) (*prometheus.CounterVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric}
	return createCounterVec(
		CacheMissCounterName, "A count of model cache miss",
		namespace, labelNames)
}

func createLoadModelCounter(namespace string) (*prometheus.CounterVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric}
	return createCounterVec(
		LoadModelCounterName, "A count of model load",
		namespace, labelNames)
}

func createUnloadModelCounter(namespace string) (*prometheus.CounterVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric}
	return createCounterVec(
		UnloadModelCounterName, "A count of model unload",
		namespace, labelNames)
}

func createLoadedModelGauge(namespace string) (*prometheus.GaugeVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric, SeldonInternalModelMetric}
	return createGaugeVec(
		LoadedModelGaugeName, "A gauge of models loaded in the system",
		namespace, labelNames)
}

func createLoadedModelMemoryGauge(namespace string) (*prometheus.GaugeVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric, SeldonInternalModelMetric}
	return createGaugeVec(
		LoadedModelMemoryGaugeName, "A gauge of models loaded memory in the system",
		namespace, labelNames)
}

func createEvictedModelMemoryGauge(namespace string) (*prometheus.GaugeVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric, SeldonInternalModelMetric}
	return createGaugeVec(
		EvictedModelMemoryGaugeName, "A gauge of models evicted from memory in the system",
		namespace, labelNames)
}

func createServerReplicaMemoryCapacityGauge(namespace string) (*prometheus.GaugeVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric}
	return createGaugeVec(
		ServerReplicaMemoryCapacityGaugeName, "A gauge of server replica memory capacity",
		namespace, labelNames)
}

func createServerReplicaMemoryCapacityWithOvercommitGauge(namespace string) (*prometheus.GaugeVec, error) {
	labelNames := []string{SeldonServerMetric, SeldonServerReplicaMetric}
	return createGaugeVec(
		ServerReplicaMemoryCapacityWithOverCommitGaugeName, "A gauge of server replica memory capacity with overcommit",
		namespace, labelNames)
}

func (pm *PrometheusMetrics) AddModelHistogramMetricsHandler(baseHandler http.HandlerFunc) http.HandlerFunc {
	handler := promhttp.InstrumentHandlerDuration(
		pm.modelHistogram.MustCurryWith(prometheus.Labels{
			SeldonServerMetric:        pm.serverName,
			SeldonServerReplicaMetric: pm.serverReplicaIdx,
		}),
		baseHandler,
	)
	return handler
}

func (pm *PrometheusMetrics) UnaryServerInterceptor() func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		startTime := time.Now()
		resp, err := handler(ctx, req)
		st, _ := status.FromError(err)
		elapsedTime := time.Since(startTime).Seconds()
		pm.modelHistogram.WithLabelValues(pm.serverName, pm.serverReplicaIdx, "grpc", st.Code().String()).Observe(elapsedTime)
		return resp, err
	}
}

func (pm *PrometheusMetrics) AddModelInferMetrics(externalModelName string, internalModelName string, method string, latency float64, code string) {
	pm.addInferLatency(externalModelName, internalModelName, method, latency, code)
	pm.addInferCount(externalModelName, internalModelName, method, code)
}

func (pm *PrometheusMetrics) AddLoadedModelMetrics(internalModelName string, memBytes uint64, isLoad, isSoft bool) {
	if isLoad {
		if isSoft {
			pm.addCacheMissCount()
			pm.addEvictedModelMemoryMetrics(internalModelName, memBytes, false) // remove it from disk
		} else {
			pm.addLoadCount()
			pm.addLoadedModelMetrics(internalModelName, isLoad)
		}
		pm.addLoadedModelMemoryMetrics(internalModelName, memBytes, isLoad)
	} else {
		if isSoft {
			pm.addCacheEvictCount()
			pm.addEvictedModelMemoryMetrics(internalModelName, memBytes, true)
		} else {
			pm.addLoadedModelMetrics(internalModelName, isLoad)
			pm.addEvictedModelMemoryMetrics(internalModelName, memBytes, false)
			pm.addUnloadCount()
		}
		pm.addLoadedModelMemoryMetrics(internalModelName, memBytes, isLoad)
	}
}

func (pm *PrometheusMetrics) AddServerReplicaMetrics(memory uint64, memoryWithOvercommit float32) {
	pm.addServerReplicaMemoryCapacityMetrics(memory, memoryWithOvercommit)
}

func (pm *PrometheusMetrics) addServerReplicaMemoryCapacityMetrics(memBytes uint64, memBytesWithOverCommit float32) {
	pm.serverReplicaMemoryCapacityGauge.With(prometheus.Labels{
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
	}).Set(float64(memBytes))
	pm.serverReplicaMemoryCapacityWithOvercommitGauge.With(prometheus.Labels{
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
	}).Set(float64(memBytesWithOverCommit))
}

func (pm *PrometheusMetrics) addLoadedModelMetrics(internalModelName string, isLoad bool) {
	if isLoad {
		pm.loadedModelGauge.With(prometheus.Labels{
			SeldonInternalModelMetric: internalModelName,
			SeldonServerMetric:        pm.serverName,
			SeldonServerReplicaMetric: pm.serverReplicaIdx,
		}).Set(1)
	} else {
		pm.loadedModelGauge.With(prometheus.Labels{
			SeldonInternalModelMetric: internalModelName,
			SeldonServerMetric:        pm.serverName,
			SeldonServerReplicaMetric: pm.serverReplicaIdx,
		}).Set(0)
	}
}

func (pm *PrometheusMetrics) addLoadedModelMemoryMetrics(internalModelName string, memBytes uint64, isLoad bool) {
	if isLoad {
		pm.loadedModelMemoryGauge.With(prometheus.Labels{
			SeldonInternalModelMetric: internalModelName,
			SeldonServerMetric:        pm.serverName,
			SeldonServerReplicaMetric: pm.serverReplicaIdx,
		}).Set(float64(memBytes))
	} else {
		pm.loadedModelMemoryGauge.With(prometheus.Labels{
			SeldonInternalModelMetric: internalModelName,
			SeldonServerMetric:        pm.serverName,
			SeldonServerReplicaMetric: pm.serverReplicaIdx,
		}).Set(0)
	}
}

func (pm *PrometheusMetrics) addEvictedModelMemoryMetrics(internalModelName string, memBytes uint64, isLoad bool) {
	// isLoad means "loaded" to disk
	if isLoad {
		pm.evictedModelMemoryGauge.With(prometheus.Labels{
			SeldonInternalModelMetric: internalModelName,
			SeldonServerMetric:        pm.serverName,
			SeldonServerReplicaMetric: pm.serverReplicaIdx,
		}).Set(float64(memBytes))
	} else {
		pm.evictedModelMemoryGauge.With(prometheus.Labels{
			SeldonInternalModelMetric: internalModelName,
			SeldonServerMetric:        pm.serverName,
			SeldonServerReplicaMetric: pm.serverReplicaIdx,
		}).Set(0)
	}
}

func (pm *PrometheusMetrics) addLoadCount() {
	pm.loadModelCounter.With(prometheus.Labels{
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
	}).Inc()
}

func (pm *PrometheusMetrics) addUnloadCount() {
	pm.unloadModelCounter.With(prometheus.Labels{
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
	}).Inc()
}

func (pm *PrometheusMetrics) addCacheMissCount() {
	pm.cacheMissCounter.With(prometheus.Labels{
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
	}).Inc()
}

func (pm *PrometheusMetrics) addCacheEvictCount() {
	pm.cacheEvictCounter.With(prometheus.Labels{
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
	}).Inc()
}

func (pm *PrometheusMetrics) addInferCount(externalModelName, internalModelName, method string, code string) {
	pm.modelInferCounter.With(prometheus.Labels{
		SeldonModelMetric:         externalModelName,
		SeldonInternalModelMetric: internalModelName,
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
		MethodTypeMetric:          method,
		CodeMetric:                code,
	}).Inc()
	pm.modelAggregateInferCounter.With(prometheus.Labels{
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
		MethodTypeMetric:          method,
	}).Inc()
}

func (pm *PrometheusMetrics) addInferLatency(externalModelName, internalModelName, method string, latency float64, code string) {
	pm.modelInferLatencyCounter.With(prometheus.Labels{
		SeldonModelMetric:         externalModelName,
		SeldonInternalModelMetric: internalModelName,
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
		MethodTypeMetric:          method,
		CodeMetric:                code,
	}).Add(latency)
	pm.modelAggregateInferLatencyCounter.With(prometheus.Labels{
		SeldonServerMetric:        pm.serverName,
		SeldonServerReplicaMetric: pm.serverReplicaIdx,
		MethodTypeMetric:          method,
	}).Add(latency)
}

func (pm *PrometheusMetrics) Start(port int) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	pm.server = &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}
	pm.logger.Infof("Starting metrics server on port %d", port)
	return pm.server.ListenAndServe()
}

func (pm *PrometheusMetrics) Stop() error {
	return pm.server.Shutdown(context.Background())
}