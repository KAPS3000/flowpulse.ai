package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/flowpulse/flowpulse/pkg/api/handlers"
	"github.com/flowpulse/flowpulse/pkg/api/middleware"
	"github.com/flowpulse/flowpulse/pkg/store/clickhouse"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type Server struct {
	cfg       *Config
	httpSrv   *http.Server
	reader    *clickhouse.Reader
	wsGateway *WSGateway
	natsConn  *nats.Conn
}

func NewServer(cfg *Config) (*Server, error) {
	reader, err := clickhouse.NewReader(cfg.ClickHouseDSN(), cfg.ClickHouse.Database)
	if err != nil {
		return nil, fmt.Errorf("create clickhouse reader: %w", err)
	}

	nc, err := nats.Connect(cfg.NATSURL())
	if err != nil {
		return nil, fmt.Errorf("connect NATS: %w", err)
	}

	wsGW, err := NewWSGateway(nc, cfg.NATS.Stream)
	if err != nil {
		return nil, fmt.Errorf("create ws gateway: %w", err)
	}

	flowH := handlers.NewFlowHandler(reader)
	metricsH := handlers.NewMetricsHandler(reader)
	topoH := handlers.NewTopologyHandler(reader)

	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(corsMiddleware)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	jwtSecret := cfg.JWTSecret()

	r.Post("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			tenantID = "local-dev"
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id":   "dev-operator",
			"tenant_id": tenantID,
			"role":      "admin",
			"iat":       time.Now().Unix(),
			"exp":       time.Now().Add(cfg.Auth.TokenExpiry).Unix(),
		})
		signed, err := token.SignedString(jwtSecret)
		if err != nil {
			http.Error(w, `{"error":"token signing failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": signed})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.JWTAuth(jwtSecret))

		r.Get("/flows", flowH.ListFlows)
		r.Get("/metrics/training", metricsH.GetTrainingMetrics)
		r.Get("/topology", topoH.GetTopology)
	})

	r.Get(cfg.API.WSPath, wsGW.HandleWebSocket)

	srv := &http.Server{
		Addr:              cfg.API.RESTListen,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return &Server{
		cfg:       cfg,
		httpSrv:   srv,
		reader:    reader,
		wsGateway: wsGW,
		natsConn:  nc,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		log.Info().Str("addr", s.cfg.API.RESTListen).Msg("REST API server listening")
		if err := s.httpSrv.ListenAndServe(); err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.wsGateway.Shutdown(shutCtx)
		s.natsConn.Close()
		s.reader.Close()
		return s.httpSrv.Shutdown(shutCtx)
	})

	return g.Wait()
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
