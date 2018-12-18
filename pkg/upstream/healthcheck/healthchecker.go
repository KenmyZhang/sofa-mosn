/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package healthcheck

import (
	"math/rand"
	"sync"
	"time"

	"github.com/alipay/sofa-mosn/pkg/api/v2"
	"github.com/alipay/sofa-mosn/pkg/log"
	"github.com/alipay/sofa-mosn/pkg/types"
)

const (
	DefaultTimeout  = time.Second
	DefaultInterval = 15 * time.Second
)

// healthChecker is a basic implementation of a health checker.
// we use different implementations of types.Session to implement different health checker
type healthChecker struct {
	//
	sessionConfig       map[string]interface{}
	cluster             types.Cluster
	sessionFactory      types.HealthCheckSessionFactory
	mutex               sync.Mutex
	checkers            map[string]*checker
	localProcessHealthy int64
	stats               *healthCheckStats
	// check config
	timeout            time.Duration
	intervalBase       time.Duration
	intervalJitter     time.Duration
	healthyThreshold   uint32
	unhealthyThreshold uint32
	rander             *rand.Rand
	hostCheckCallbacks []types.HealthCheckCb
}

func newHealthChecker(cfg v2.HealthCheck, cluster types.Cluster, f types.HealthCheckSessionFactory) types.HealthChecker {
	timeout := DefaultTimeout
	if cfg.Timeout != 0 {
		timeout = cfg.Timeout
	}
	interval := DefaultInterval
	if cfg.Interval != 0 {
		interval = cfg.Interval
	}
	hc := &healthChecker{
		// cfg
		sessionConfig:      cfg.SessionConfig,
		timeout:            timeout,
		intervalBase:       interval,
		intervalJitter:     cfg.IntervalJitter,
		healthyThreshold:   cfg.HealthyThreshold,
		unhealthyThreshold: cfg.UnhealthyThreshold,
		//runtime and stats
		cluster:            cluster,
		rander:             rand.New(rand.NewSource(time.Now().UnixNano())),
		hostCheckCallbacks: []types.HealthCheckCb{},
		sessionFactory:     f,
		mutex:              sync.Mutex{},
		checkers:           make(map[string]*checker),
		stats:              newHealthCheckStats(cfg.ServiceName),
	}
	// Add common callbacks when create
	// common callbacks should be registered and configured
	for _, name := range cfg.CommonCallbacks {
		if cb, ok := commonCallbacks[name]; ok {
			hc.AddHostCheckCompleteCb(cb)
		}
	}
	return hc
}

func (hc *healthChecker) Start() {
	for _, hostSet := range hc.cluster.PrioritySet().HostSetsByPriority() {
		hosts := hostSet.Hosts()
		for _, h := range hosts {
			hc.startCheck(h)
		}
	}
	hc.stats.healthy.Update(hc.localProcessHealthy)
}

func (hc *healthChecker) Stop() {
	for _, hostSet := range hc.cluster.PrioritySet().HostSetsByPriority() {
		hosts := hostSet.Hosts()
		for _, h := range hosts {
			hc.stopCheck(h)
		}
	}
}

func (hc *healthChecker) AddHostCheckCompleteCb(cb types.HealthCheckCb) {
	hc.hostCheckCallbacks = append(hc.hostCheckCallbacks, cb)
}

func (hc *healthChecker) OnClusterMemberUpdate(hostsAdd []types.Host, hostsDel []types.Host) {
	for _, h := range hostsAdd {
		hc.startCheck(h)
	}
	for _, h := range hostsDel {
		hc.stopCheck(h)
	}
	hc.stats.healthy.Update(hc.localProcessHealthy)
}

func (hc *healthChecker) startCheck(host types.Host) {
	addr := host.AddressString()
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	if _, ok := hc.checkers[addr]; !ok {
		s := hc.sessionFactory.NewSession(hc.sessionConfig, host)
		if s == nil {
			log.DefaultLogger.Errorf("Create Health Check Session Error, Remote Address = %s", addr)
			return
		}
		c := newChecker(s, host, hc)
		hc.checkers[addr] = c
		go c.Start()
		hc.localProcessHealthy++ // default host is healthy
		log.DefaultLogger.Infof("create a health check session for %s", addr)
	}
}

func (hc *healthChecker) stopCheck(host types.Host) {
	addr := host.AddressString()
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	if c, ok := hc.checkers[addr]; ok {
		c.Stop()
		delete(hc.checkers, addr)
		hc.localProcessHealthy-- // deleted check is unhealthy
		log.DefaultLogger.Infof("remove a health check session for %s", addr)
	}
}

func (hc *healthChecker) runCallbacks(host types.Host, changed bool, isHealthy bool) {
	hc.stats.healthy.Update(hc.localProcessHealthy)
	for _, cb := range hc.hostCheckCallbacks {
		cb(host, changed, isHealthy)
	}
}

func (hc *healthChecker) getCheckInterval() time.Duration {
	interval := hc.intervalBase
	if hc.intervalJitter > 0 {
		interval += time.Duration(hc.rander.Int63n(int64(hc.intervalJitter)))
	}
	// TODO: support jitter percentage
	return interval
}

func (hc *healthChecker) incHealthy(host types.Host, changed bool) {
	hc.stats.success.Inc(1)
	if changed {
		hc.localProcessHealthy++
	}
	hc.runCallbacks(host, changed, true)
}

func (hc *healthChecker) decHealthy(host types.Host, reason types.FailureType, changed bool) {
	hc.stats.failure.Inc(1)
	if changed {
		hc.localProcessHealthy--
	}
	switch reason {
	case types.FailureActive:
		hc.stats.activeFailure.Inc(1)
	case types.FailureNetwork:
		hc.stats.networkFailure.Inc(1)
	case types.FailurePassive: //TODO: not support yet
		hc.stats.passiveFailure.Inc(1)
	}
	hc.runCallbacks(host, changed, false)

}
