package client

import (
	"fmt"
	"github.com/shitamachi/loki-client/api"
	"github.com/shitamachi/loki-client/logproto"
	"go.uber.org/zap"
	"os"
	"sync"
	"time"

	"github.com/joncrlsn/dque"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
)

type DqueConfig struct {
	QueueDir         string `json:"queue_dir"`
	QueueSegmentSize int    `json:"queue_segment_size"`
	QueueSync        bool   `json:"queue_sync"`
	QueueName        string `json:"queue_name"`
}

var DefaultDqueConfig = DqueConfig{
	QueueDir:         "./tmp/loki-dque-storage/default",
	QueueSegmentSize: 500,
	QueueSync:        false,
	QueueName:        "dque",
}

type dqueEntry struct {
	Lbs  model.LabelSet
	Ts   time.Time
	Line string
}

func dqueEntryBuilder() interface{} {
	return &dqueEntry{}
}

type dqueClient struct {
	logger  *zap.Logger
	queue   *dque.DQue
	loki    Client
	once    sync.Once
	wg      sync.WaitGroup
	entries chan api.Entry
}

type BufferedClientConfig struct {
	DqueConfig   *DqueConfig
	ClientConfig Config
}

// NewDqueClient New makes a new dque loki client
func NewDqueClient(cfg *BufferedClientConfig, logger *zap.Logger) (Client, error) {
	var err error

	if cfg.DqueConfig == nil {
		cfg.DqueConfig = &DefaultDqueConfig
	}

	q := &dqueClient{
		logger: logger.With(zap.String("component", "queue"), zap.String("name", cfg.DqueConfig.QueueName)),
	}

	err = os.MkdirAll(cfg.DqueConfig.QueueDir, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("cannot create queue directory: %s", err)
	}

	q.queue, err = dque.NewOrOpen(cfg.DqueConfig.QueueName, cfg.DqueConfig.QueueDir, cfg.DqueConfig.QueueSegmentSize, dqueEntryBuilder)
	if err != nil {
		return nil, err
	}

	if !cfg.DqueConfig.QueueSync {
		_ = q.queue.TurboOn()
	}

	q.loki, err = New(prometheus.DefaultRegisterer, cfg.ClientConfig, logger)
	if err != nil {
		return nil, err
	}

	q.entries = make(chan api.Entry)

	q.wg.Add(2)
	go q.enqueuer()
	go q.dequeuer()
	return q, nil
}

func (c *dqueClient) dequeuer() {
	defer c.wg.Done()
	for {
		// Dequeue the next item in the queue
		entry, err := c.queue.DequeueBlock()
		if err != nil {
			switch err {
			case dque.ErrQueueClosed:
				return
			default:
				c.logger.Error("error dequeuing record", zap.Error(err))
				continue
			}
		}

		// Assert type of the response to an Item pointer so we can work with it
		record, ok := entry.(*dqueEntry)
		if !ok {
			c.logger.Error("error dequeued record is not an valid type")
			continue
		}

		c.loki.Chan() <- api.Entry{
			Labels: record.Lbs,
			Entry: logproto.Entry{
				Timestamp: record.Ts,
				Line:      record.Line,
			},
		}
	}
}

// Stop the client
func (c *dqueClient) Stop() {
	c.once.Do(func() {
		close(c.entries)
		_ = c.queue.Close()
		c.loki.Stop()
		c.wg.Wait()
	})

}

func (c *dqueClient) Chan() chan<- api.Entry {
	return c.entries
}

// StopNow Stop the client
func (c *dqueClient) StopNow() {
	c.once.Do(func() {
		close(c.entries)
		_ = c.queue.Close()
		c.loki.StopNow()
		c.wg.Wait()
	})
}

func (c *dqueClient) enqueuer() {
	defer c.wg.Done()
	for e := range c.entries {
		if err := c.queue.Enqueue(&dqueEntry{e.Labels, e.Timestamp, e.Line}); err != nil {
			c.logger.Warn(fmt.Sprintf("cannot enqueue record %s:", e.Line), zap.Error(err))
		}
	}
}
