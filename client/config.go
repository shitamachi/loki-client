package client

import (
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"net/url"
	"time"
)

const (
	BatchWait      = 1 * time.Second
	BatchSize  int = 1024 * 1024
	MinBackoff     = 500 * time.Millisecond
	MaxBackoff     = 5 * time.Minute
	MaxRetries int = 10
	Timeout        = 10 * time.Second
)

type Config struct {
	URL       *url.URL
	BatchWait time.Duration
	BatchSize int

	Client config.HTTPClientConfig

	BackoffConfig BackoffConfig
	// The labels to add to any time series or alerts when communicating with loki
	ExternalLabels model.LabelSet
	Timeout        time.Duration

	// The tenant ID to use when pushing logs to Loki (empty string means
	// single tenant mode)
	TenantID string `yaml:"tenant_id"`
}
