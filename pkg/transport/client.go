package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/flowpulse/flowpulse/pkg/model"
)

type GRPCClient struct {
	httpClient *http.Client
	baseURL    string
}

func NewGRPCClient(addr string) (*GRPCClient, error) {
	return &GRPCClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    fmt.Sprintf("http://%s", addr),
	}, nil
}

func (c *GRPCClient) SendFlows(batch *model.FlowBatch) error {
	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v1/ingest/flows",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("POST ingest/flows: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("ingest returned %d", resp.StatusCode)
	}
	return nil
}

func (c *GRPCClient) SendMetrics(metrics *model.NodeMetrics) error {
	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v1/ingest/metrics",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("POST ingest/metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("ingest returned %d", resp.StatusCode)
	}
	return nil
}

func (c *GRPCClient) Close() error {
	return nil
}
