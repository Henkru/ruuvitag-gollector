package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"time"

	"github.com/niktheblak/ruuvitag-gollector/pkg/exporter"
	"github.com/niktheblak/ruuvitag-gollector/pkg/sensor"
)

type httpExporter struct {
	client *nethttp.Client
	url    string
	token  string
}

func New(url, token string, timeout time.Duration) (exporter.Exporter, error) {
	if url == "" {
		return nil, fmt.Errorf("parameter url must be non-empty")
	}
	client := &nethttp.Client{
		Timeout: timeout,
	}
	return &httpExporter{
		client: client,
		url:    url,
		token:  token,
	}, nil
}

func (h httpExporter) Name() string {
	return fmt.Sprintf("HTTP (%s)", h.url)
}

func (h httpExporter) Export(ctx context.Context, data ...sensor.Data) error {
	if len(data) == 0 {
		return exporter.ErrNoMeasurements
	}
	for _, m := range data {
		buf := new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		err := enc.Encode(m)
		if err != nil {
			return err
		}
		req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, h.url, buf)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("From", "ruuvitag-gollector")
		if h.token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.token))
		}
		resp, err := h.client.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (h httpExporter) Close() error {
	h.client.CloseIdleConnections()
	return nil
}
