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
	"github.com/alipay/sofa-mosn/pkg/api/v2"
	"github.com/alipay/sofa-mosn/pkg/types"
)

var sessionFactories map[types.Protocol]types.HealthCheckSessionFactory

func init() {
	sessionFactories = make(map[types.Protocol]types.HealthCheckSessionFactory)
}

func RegisterSessionFactory(p types.Protocol, f types.HealthCheckSessionFactory) {
	sessionFactories[p] = f
}

// CreateHealthCheck is a extendable function that can create different health checker
// by different health check session.
// The Default session is TCPDial session
func CreateHealthCheck(cfg v2.HealthCheck, cluster types.Cluster) types.HealthChecker {
	f, ok := sessionFactories[types.Protocol(cfg.Protocol)]
	if !ok {
		// not registered, use default session factory
		f = &TCPDialSessionFactory{}
	}
	return newHealthChecker(cfg, cluster, f)
}
