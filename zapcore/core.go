package zapcore

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/shitamachi/loki-client/api"
	client2 "github.com/shitamachi/loki-client/client"
	"github.com/shitamachi/loki-client/logproto"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/url"
	"time"
)

type LokiCoreConfig struct {
	URL            string              `json:"url"`
	SendLevel      int8                `json:"send_level"`
	BatchWait      int64               `json:"batch_wait"`
	BatchSize      int                 `json:"batch_size"`
	TenantID       string              `json:"tenant_id"`
	ExternalLabels model.LabelSet      `json:"external_labels"`
	BufferedClient bool                `json:"buffered_client"`
	BufferedConfig *client2.DqueConfig `json:"buffered_config"`
}

type LokiCore struct {
	config *LokiCoreConfig
	client client2.Client
	zapcore.LevelEnabler
	enc zapcore.Encoder
}

func NewLokiCore(cfg *LokiCoreConfig) (*LokiCore, error) {
	var client client2.Client

	if cfg == nil {
		cfg = &LokiCoreConfig{}
	}
	parse, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, err
	}
	clientCfg := client2.Config{
		URL:       parse,
		BatchWait: time.Duration(cfg.BatchWait) * time.Millisecond,
		BatchSize: cfg.BatchSize,
		Client:    config.HTTPClientConfig{},
		BackoffConfig: client2.BackoffConfig{
			MinBackoff: 100 * time.Millisecond,
			MaxBackoff: 10 * time.Second,
			MaxRetries: 10,
		},
		ExternalLabels: cfg.ExternalLabels,
		Timeout:        5 * time.Second,
		TenantID:       cfg.TenantID,
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}

	if cfg.BufferedClient {
		client, err = client2.NewDqueClient(&client2.BufferedClientConfig{
			DqueConfig:   cfg.BufferedConfig,
			ClientConfig: clientCfg,
		}, logger)
	} else {
		client, err = client2.New(prometheus.DefaultRegisterer, clientCfg, logger)
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}

	return &LokiCore{
		config:       cfg,
		client:       client,
		LevelEnabler: zapcore.Level(cfg.SendLevel),
		enc:          zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
	}, nil
}

func (c *LokiCoreConfig) Default() {
	*c = LokiCoreConfig{
		URL:       "",
		SendLevel: 0,
		BatchWait: 0,
		BatchSize: 0,
		TenantID:  "",
	}
}

func (c *LokiCore) With(fields []zapcore.Field) zapcore.Core {
	clone := c.clone()
	for i := range fields {
		fields[i].AddTo(clone.enc)
	}
	return clone
}

func (c *LokiCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *LokiCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	buf, err := c.enc.EncodeEntry(entry, fields)
	if err != nil {
		return err
	}
	labels := model.LabelSet{
		"level": model.LabelValue(entry.Level.String()),
	}
	e := api.Entry{
		Labels: labels.Merge(c.config.ExternalLabels),
		Entry: logproto.Entry{
			Timestamp: time.Now(),
			Line:      buf.String(),
		},
	}
	c.client.Chan() <- e
	buf.Free()
	if entry.Level > zapcore.ErrorLevel {
		// Since we may be crashing the program, sync the output. Ignore Sync
		// errors, pending a clean solution to issue #370.
		err := c.Sync()
		if err != nil {
			fmt.Printf("got error %v", err)
		}
	}
	return nil
}

func (c *LokiCore) Sync() error {
	return nil
}

func (c *LokiCore) clone() *LokiCore {
	return &LokiCore{
		config:       c.config,
		client:       c.client,
		LevelEnabler: c.LevelEnabler,
		enc:          c.enc.Clone(),
	}
}
