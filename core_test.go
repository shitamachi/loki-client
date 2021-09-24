package main

import (
	"fmt"
	"github.com/prometheus/common/model"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"math/rand"
	"testing"
	"time"
)

func TestLokiCore_Write(t *testing.T) {
	lokiCore, err := NewLokiCore(&LokiCoreConfig{
		URL:       "http://127.0.0.1:3100/loki/api/v1/push",
		SendLevel: 0,
		BatchWait: 3 * time.Second,
		BatchSize: 5,
		TenantID:  "test-1",
	})
	if err != nil {
		t.Error(err)
	}

	newTee := zapcore.NewTee(zap.L().Core(), lokiCore)
	zap.ReplaceGlobals(zap.New(newTee))

	log := zap.L()

	for i := 0; i < 100; i++ {
		t.Logf("invoke %d log", i)
		randInt := rand.Intn(3)
		t.Logf("sleep %d sec", randInt)
		time.Sleep(time.Duration(randInt) * time.Second)

		if ce := log.Check(zapcore.InfoLevel, fmt.Sprintf("test log %d", i)); ce != nil {
			ce.Write(zap.Reflect("time", time.Now()), zap.Int("index", i))
		}
	}
}

func TestMultipleWrite(t *testing.T) {
	lokiCore, err := NewLokiCore(&LokiCoreConfig{
		URL:       "http://127.0.0.1:3100/loki/api/v1/push",
		SendLevel: 0,
		BatchWait: 3 * time.Second,
		BatchSize: 5,
		TenantID:  "test-m-1",
		ExternalLabels: map[model.LabelName]model.LabelValue{
			"source":   "test",
			"instance": "test-m-1",
		},
	})
	if err != nil {
		t.Error(err)
	}

	lokiCore2, err := NewLokiCore(&LokiCoreConfig{
		URL:       "http://127.0.0.1:3100/loki/api/v1/push",
		SendLevel: 0,
		BatchWait: 3 * time.Second,
		BatchSize: 5,
		TenantID:  "test-m-2",
		ExternalLabels: map[model.LabelName]model.LabelValue{
			"source":   "test",
			"instance": "test-m-2",
		},
	})
	if err != nil {
		t.Error(err)
	}

	log, err := zap.NewDevelopment()
	if err != nil {
		t.Error(err)
	}
	log = log.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, lokiCore)
	}))

	log2, err := zap.NewDevelopment()
	if err != nil {
		t.Error(err)
	}
	log2 = log2.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, lokiCore2)
	}))

	go func() {
		for i := 0; i < 100; i++ {
			fmt.Printf("invoke log1 %d log\n", i)

			if ce := log.Check(zapcore.InfoLevel, fmt.Sprintf("test log %d", i)); ce != nil {
				ce.Write(zap.Reflect("time", time.Now()), zap.Int("index", i))
			}
		}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			fmt.Printf("invoke log2 %d log\n", i)

			if ce := log2.Check(zapcore.WarnLevel, fmt.Sprintf("test log %d", i)); ce != nil {
				ce.Write(zap.Reflect("time", time.Now()), zap.Int("index", i))
			}
		}
	}()

	time.Sleep(15 * time.Second)
}
