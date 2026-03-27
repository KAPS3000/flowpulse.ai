package aggregator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/flowpulse/flowpulse/pkg/model"
	"github.com/flowpulse/flowpulse/pkg/transport"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

// FlowWriter abstracts persistent storage for correlated flows, node metrics, and training metrics.
type FlowWriter interface {
	WriteFlows(ctx context.Context, flows []*model.CorrelatedFlow) error
	WriteNodeMetrics(ctx context.Context, metrics []model.NodeMetrics) error
	WriteTrainingMetrics(ctx context.Context, tm *model.TrainingMetrics) error
	Close() error
}

type Aggregator struct {
	cfg        *Config
	grpcServer *transport.GRPCServer
	correlator *FlowCorrelator
	metrics    *MetricsComputer
	chWriter   FlowWriter
	natsConn   *nats.Conn
	natsJS     nats.JetStreamContext

	flowCh   chan model.FlowBatch
	metricCh chan model.NodeMetrics

	nodeMetrics map[string]model.NodeMetrics
}

func New(cfg *Config, writer FlowWriter) (*Aggregator, error) {
	grpcSrv, err := transport.NewGRPCServer(cfg.Aggregator.GRPCListen)
	if err != nil {
		return nil, fmt.Errorf("create gRPC server: %w", err)
	}

	nc, err := nats.Connect(cfg.NATSURL())
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	js, err := nc.JetStream(nats.PublishAsyncMaxPending(cfg.NATS.MaxPending))
	if err != nil {
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     cfg.NATS.Stream,
		Subjects: []string{cfg.NATS.Stream + ".>"},
		Storage:  nats.MemoryStorage,
		MaxAge:   5 * time.Minute,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to create NATS stream (may already exist)")
	}

	return &Aggregator{
		cfg:         cfg,
		grpcServer:  grpcSrv,
		correlator:  NewFlowCorrelator(),
		metrics:     NewMetricsComputer(0),
		chWriter:    writer,
		natsConn:    nc,
		natsJS:      js,
		flowCh:      make(chan model.FlowBatch, 4096),
		metricCh:    make(chan model.NodeMetrics, 1024),
		nodeMetrics: make(map[string]model.NodeMetrics),
	}, nil
}

func (a *Aggregator) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.grpcServer.Serve()
	})

	g.Go(func() error {
		return a.serveHTTP(ctx)
	})

	g.Go(func() error {
		return a.processFlows(ctx)
	})

	g.Go(func() error {
		return a.processMetrics(ctx)
	})

	g.Go(func() error {
		return a.flushLoop(ctx)
	})

	g.Go(func() error {
		return a.computeMetricsLoop(ctx)
	})

	g.Go(func() error {
		<-ctx.Done()
		a.grpcServer.GracefulStop()
		a.natsConn.Close()
		return nil
	})

	return g.Wait()
}

func (a *Aggregator) serveHTTP(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/ingest/flows", a.handleIngestFlows)
	mux.HandleFunc("/api/v1/ingest/metrics", a.handleIngestMetrics)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{Addr: ":9092", Handler: mux}
	log.Info().Str("addr", ":9092").Msg("HTTP ingest server listening")

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP ingest server: %w", err)
	}
	return nil
}

func (a *Aggregator) handleIngestFlows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024))
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	var batch model.FlowBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	a.IngestFlowBatch(batch)
	log.Debug().Str("node", batch.NodeID).Int("flows", len(batch.Flows)).Msg("ingested flow batch via HTTP")

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"accepted":true}`))
}

func (a *Aggregator) handleIngestMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	var metrics model.NodeMetrics
	if err := json.Unmarshal(body, &metrics); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	a.IngestNodeMetrics(metrics)
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"accepted":true}`))
}

// IngestFlowBatch is called by gRPC handler when a batch arrives from an agent.
func (a *Aggregator) IngestFlowBatch(batch model.FlowBatch) {
	select {
	case a.flowCh <- batch:
	default:
		log.Warn().
			Str("node", batch.NodeID).
			Int("flows", len(batch.Flows)).
			Msg("aggregator flow channel full, dropping batch")
	}
}

// IngestNodeMetrics is called by gRPC handler when metrics arrive.
func (a *Aggregator) IngestNodeMetrics(metrics model.NodeMetrics) {
	select {
	case a.metricCh <- metrics:
	default:
		log.Warn().Str("node", metrics.NodeID).Msg("metric channel full, dropping")
	}
}

func (a *Aggregator) processFlows(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case batch := <-a.flowCh:
			for _, flow := range batch.Flows {
				a.correlator.Ingest(flow)
			}

			// Publish real-time event to NATS
			subject := fmt.Sprintf("%s.%s.flows", a.cfg.NATS.Stream, batch.TenantID)
			data, err := json.Marshal(batch)
			if err == nil {
				if _, pubErr := a.natsJS.Publish(subject, data); pubErr != nil {
					log.Warn().Err(pubErr).Str("subject", subject).Msg("NATS publish failed")
				}
			}
		}
	}
}

func (a *Aggregator) processMetrics(ctx context.Context) error {
	flushTicker := time.NewTicker(5 * time.Second)
	defer flushTicker.Stop()
	var pendingMetrics []model.NodeMetrics

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-flushTicker.C:
			if len(pendingMetrics) > 0 {
				if err := a.chWriter.WriteNodeMetrics(ctx, pendingMetrics); err != nil {
					log.Error().Err(err).Msg("ClickHouse node_metrics write failed")
				} else {
					log.Debug().Int("count", len(pendingMetrics)).Msg("flushed node metrics to ClickHouse")
				}
				pendingMetrics = nil
			}
		case m := <-a.metricCh:
			a.nodeMetrics[m.NodeID] = m
			pendingMetrics = append(pendingMetrics, m)

			subject := fmt.Sprintf("%s.%s.metrics", a.cfg.NATS.Stream, m.TenantID)
			data, err := json.Marshal(m)
			if err == nil {
				if _, pubErr := a.natsJS.Publish(subject, data); pubErr != nil {
					log.Warn().Err(pubErr).Msg("NATS metrics publish failed")
				}
			}
		}
	}
}

func (a *Aggregator) flushLoop(ctx context.Context) error {
	ticker := time.NewTicker(a.cfg.Aggregator.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			flows := a.correlator.DrainAll()
			if len(flows) == 0 {
				continue
			}

			if err := a.chWriter.WriteFlows(ctx, flows); err != nil {
				log.Error().Err(err).Int("count", len(flows)).Msg("ClickHouse write failed")
			} else {
				log.Info().Int("count", len(flows)).Msg("flushed flows to ClickHouse")
			}
		}
	}
}

func (a *Aggregator) computeMetricsLoop(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if len(a.nodeMetrics) == 0 {
				continue
			}

			// Group by tenant
			tenantNodes := make(map[string][]model.NodeMetrics)
			for _, nm := range a.nodeMetrics {
				tenantNodes[nm.TenantID] = append(tenantNodes[nm.TenantID], nm)
			}

			flows := a.correlator.DrainAll()

			for tenantID, nodes := range tenantNodes {
				tm := a.metrics.ComputeTrainingMetrics(tenantID, nodes, flows, 5*time.Second)

				if err := a.chWriter.WriteTrainingMetrics(ctx, &tm); err != nil {
					log.Error().Err(err).Msg("ClickHouse training_metrics write failed")
				}

				subject := fmt.Sprintf("%s.%s.training", a.cfg.NATS.Stream, tenantID)
				data, err := json.Marshal(tm)
				if err == nil {
					if _, pubErr := a.natsJS.Publish(subject, data); pubErr != nil {
						log.Warn().Err(pubErr).Msg("training metrics publish failed")
					}
				}

				log.Debug().
					Str("tenant", tenantID).
					Float64("straggler", tm.StragglerScore).
					Float64("bubble", tm.BubbleRatio).
					Float64("saturation", tm.NetworkSaturationIndex).
					Msg("computed training metrics")
			}
		}
	}
}
