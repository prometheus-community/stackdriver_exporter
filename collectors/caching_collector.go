package collectors

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"sync"
	"time"
)

type CachingCollector struct {
	collector       prometheus.Collector
	refreshInterval *time.Duration
	cache           []prometheus.Metric
	mux             sync.RWMutex
}

func NewCachingCollector(collector prometheus.Collector, refreshInterval *time.Duration,
) (*CachingCollector, error) {
	cachingCollector := &CachingCollector{
		collector:       collector,
		refreshInterval: refreshInterval,
		cache:           make([]prometheus.Metric, 0),
	}

	updateCache(cachingCollector)
	go periodicallyUpdateCache(cachingCollector)

	return cachingCollector, nil
}

func (c *CachingCollector) Describe(ch chan<- *prometheus.Desc) {
	c.collector.Describe(ch)
}

func (c *CachingCollector) Collect(ch chan<- prometheus.Metric) {
	c.mux.RLock()
	for _, element := range c.cache {
		ch <- element
	}
	c.mux.RUnlock()
}

func periodicallyUpdateCache(c *CachingCollector) {
	for range time.Tick(*c.refreshInterval) {
		updateCache(c)
	}
}

func updateCache(c *CachingCollector) {
	log.Debug("Updating cache")
	ch := make(chan prometheus.Metric)
	newCache := make([]prometheus.Metric, 0)

	m := &sync.Mutex{}
	m.Lock()
	go func() {
		m.Lock()
		close(ch)
	}()
	go func(collector prometheus.Collector) {
		defer m.Unlock()
		collector.Collect(ch)
	}(c.collector)

	for metric := range ch {
		newCache = append(newCache, metric)
	}

	c.mux.Lock()
	c.cache = newCache
	c.mux.Unlock()
	log.Debug("Updated cache")
}
