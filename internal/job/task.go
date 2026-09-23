// Package job defines the platform's background tasks and their handlers.
// Every task is idempotent, bounded, isolated and recorded.
package job

import (
	"log/slog"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/hibiken/asynq"
	goredis "github.com/redis/go-redis/v9"
)

// Task type names. Later phases add integration, billing, alarm and report tasks.
const (
	TypeNoop = "system.noop"
)

// Queue names, ordered by priority when the worker picks work.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// RedisOpt builds the asynq connection options, pointing at the queue database.
func RedisOpt(cfg config.Redis) (asynq.RedisClientOpt, error) {
	opts, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return asynq.RedisClientOpt{}, secret.URLParseErr("parse redis url")
	}
	return asynq.RedisClientOpt{
		Addr:     opts.Addr,
		Username: opts.Username,
		Password: opts.Password,
		DB:       cfg.QueueDB,
	}, nil
}

// Handlers holds the dependencies every task handler needs. The
// integration fields are optional: a process that never runs the
// integration tasks (for example a test harness, or a worker deliberately
// split off by task type) simply leaves them nil, and Register registers
// no handler for the task types they would have served.
type Handlers struct {
	Log *slog.Logger

	Ingestion          Ingestion
	Backfill           Backfiller
	Prices             PriceSyncer
	ConsumptionRefresh Refresher
	AnalyzerRefresh    AnalyzerRefresher
	Demo               DemoExtender

	BillingDispatch BillingDispatcher
	BillingGenerate BillingGenerator
	BillingRender   BillingRenderer

	AlarmDispatch AlarmDispatcher
	AlarmEvaluate AlarmEvaluator
	AlarmNotify   AlarmNotifier

	ReportDispatch ReportDispatcher
	ReportGenerate ReportGenerator
	ReportDeliver  ReportDeliverer

	// Solar serves the iSolar sync and fault jobs (R288).
	Solar SolarJobs
	// Carbon serves the daily carbon accrual (R311).
	Carbon CarbonJobs
}

// Register attaches every handler to the mux. TypeNoop is always
// registered. Each integration task type is registered only when the
// Handlers field that serves it is non-nil, through an adapter that
// decodes the payload, calls the method, and returns
// ClassifyForRetry(err) — so a worker process that was not given an
// Ingestion, Backfiller or PriceSyncer simply has no route for the task
// types they would have served, rather than panicking on a nil interface
// the first time one arrives.
func Register(mux *asynq.ServeMux, h *Handlers) {
	mux.HandleFunc(TypeNoop, h.Noop)

	if h.Ingestion != nil {
		mux.HandleFunc(TypeIntegrationSyncDispatch, h.integHandleSyncDispatch)
		mux.HandleFunc(TypeIntegrationSyncAnalyzers, h.integHandleSyncAnalyzers)
		mux.HandleFunc(TypeIntegrationFetchReadings, h.integHandleFetchReadings)
	}
	if h.Demo != nil {
		mux.HandleFunc(TypeDemoExtend, h.handleDemoExtend)
	}
	if h.AnalyzerRefresh != nil {
		mux.HandleFunc(TypeIntegrationRefreshAnalyzer, h.handleRefreshAnalyzer)
	}
	if h.Backfill != nil {
		mux.HandleFunc(TypeIntegrationBackfill, h.integHandleBackfill)
	}
	if h.Prices != nil {
		mux.HandleFunc(TypeEPIASSyncPrices, h.integHandleSyncPrices)
	}
	if h.ConsumptionRefresh != nil {
		mux.HandleFunc(TypeConsumptionRefresh, h.integHandleConsumptionRefresh)
	}
	if h.BillingDispatch != nil {
		mux.HandleFunc(TypeBillingDispatch, h.handleBillingDispatch)
	}
	if h.BillingGenerate != nil {
		mux.HandleFunc(TypeBillingGenerate, h.handleBillingGenerate)
	}
	if h.BillingRender != nil {
		mux.HandleFunc(TypeBillingRenderPDF, h.handleBillingRender)
	}
	if h.AlarmDispatch != nil {
		mux.HandleFunc(TypeAlarmDispatch, h.handleAlarmDispatch)
	}
	if h.AlarmEvaluate != nil {
		mux.HandleFunc(TypeAlarmEvaluate, h.handleAlarmEvaluate)
	}
	if h.AlarmNotify != nil {
		mux.HandleFunc(TypeAlarmNotify, h.handleAlarmNotify)
	}
	if h.Solar != nil {
		mux.HandleFunc(TypeSolarDispatchSync, h.handleSolarDispatch)
		mux.HandleFunc(TypeSolarSyncPlant, h.handleSolarSync)
		mux.HandleFunc(TypeSolarFetchAlarms, h.handleSolarAlarms)
	}
	if h.Carbon != nil {
		mux.HandleFunc(TypeCarbonAccrual, h.handleCarbonAccrual)
	}
	if h.ReportDispatch != nil {
		mux.HandleFunc(TypeReportDispatchMonthly, h.handleReportDispatchMonthly)
		mux.HandleFunc(TypeReportDispatchYearly, h.handleReportDispatchYearly)
	}
	if h.ReportGenerate != nil {
		mux.HandleFunc(TypeReportGenerate, h.handleReportGenerate)
	}
	if h.ReportDeliver != nil {
		mux.HandleFunc(TypeReportDeliver, h.handleReportDeliver)
	}
}
