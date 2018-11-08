// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package metrics contains definitions of monitored metrics of cloudsql-proxy.
package metrics

import (
	"fmt"
	"math"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	octag "go.opencensus.io/tag"
)

// Metric represents a metric to be tracked and exported.
//
// Note on string valued metric: Since opencensus does not support string value metrics, we use
// tag with StrValKey as an indicator for it. If StrValKey is present in a metric's key list, the
// metric is considered as having string value type and the value will be stored in tag with
// StrValKey. In this case, aggregation type must be view.AggTypeLastValue because string valued
// metrics are naturally gauge type and we make it a rule that Measure must store int64 value
// although the value stored in Measure is meaningless.
type Metric struct {
	// Measure is the opencensus stats.Measure object tracked for this metric.
	Measure stats.Measure
	// Agg represents the data aggregation method for this metric.
	Agg *view.Aggregation
	// Keys are opencensus keys specific to this metric.
	Keys []octag.Key
}

func (m *Metric) String() string {
	return m.Measure.Name()
}

// ValueType represents the logical type of metric values.
type ValueType int

const (
	// ValInvalid represents invalid value type. No well-formed metric should have this value.
	ValInvalid ValueType = iota
	// ValCount represents a counter value.
	ValCount
	// ValInt represents an integer value.
	ValInt
	// ValDuration represents a duration value.
	ValDuration
	// ValString represents a string value.
	ValString
)

func (v ValueType) String() string {
	switch v {
	case ValInvalid:
		return "invalid"
	case ValCount:
		return "count"
	case ValInt:
		return "int"
	case ValDuration:
		return "duration"
	case ValString:
		return "string"
	default:
		return fmt.Sprintf("ValueType(%d)", int(v))
	}
}

// ValType returns the logical value type of the metric.
func (m *Metric) ValType() ValueType {
	// String valued metric is identified by the presence of the key StrValKey.
	for _, key := range m.Keys {
		if key == tag.StrValKey {
			return ValString
		}
	}

	if m.Agg.Type == view.AggTypeCount {
		return ValCount
	}

	switch m.Measure.(type) {
	case *stats.Int64Measure:
		return ValInt
	case *stats.Float64Measure:
		// As of now, all float values represents duration.
		return ValDuration
	default:
		return ValInvalid
	}
}

var (
	// AdminAPIReqCount represents the count of SQL admin API calls.
	AdminAPIReqCount = &Metric{
		Measure: stats.Int64(
			"cloudsql.googleapis.com/database/proxy/client/admin_api_request_count",
			"Count of CLoud SQL Admin API calls.",
			stats.UnitDimensionless,
		),
		Agg:  view.Count(),
		Keys: []octag.Key{tag.APIMethodKey, tag.RespCodeKey},
	}

	// ActiveConnNum represents the number of active connections.
	ActiveConnNum = &Metric{
		Measure: stats.Int64(
			"cloudsql.googleapis.com/database/proxy/client/connection/active",
			"Count of active connections serviced by the proxy client.",
			stats.UnitDimensionless,
		),
		Agg:  view.LastValue(),
		Keys: []octag.Key{tag.IPTypeKey},
	}

	// ConnLatency represents the latency of connections.
	ConnLatency = &Metric{
		Measure: stats.Float64(
			"cloudsql.googleapis.com/database/proxy/client/connection/latencies",
			"Latency of proxy client connections.",
			stats.UnitMilliseconds,
		),
		Agg:  view.Distribution(makeExpBucket(1, 1.5, 22)...),
		Keys: []octag.Key{tag.IPTypeKey},
	}

	// ConnReqCount respresnts the count of connection requests.
	ConnReqCount = &Metric{
		Measure: stats.Int64(
			"cloudsql.googleapis.com/database/proxy/client/connection/request_count",
			"Count of connection requests serviced by the proxy client.",
			stats.UnitDimensionless,
		),
		Agg: view.Count(),
	}

	// TermCodeCount represents the count of connection termination codes.
	TermCodeCount = &Metric{
		Measure: stats.Int64(
			"cloudsql.googleapis.com/database/proxy/client/connection/termination_code_count",
			"Count of terminated connections.",
			stats.UnitDimensionless,
		),
		Agg:  view.Count(),
		Keys: []octag.Key{tag.IPTypeKey, tag.TermCodeKey},
	}

	// RcvBytes represents count of bytes received by the proxy client.
	RcvBytes = &Metric{
		Measure: stats.Int64(
			"cloudsql.googleapis.com/database/proxy/client/received_bytes_count",
			"Count of bytes received.",
			stats.UnitBytes,
		),
		Agg:  view.Sum(),
		Keys: []octag.Key{tag.IPTypeKey},
	}

	// SentBytes represents count of bytes sent from the proxy client.
	SentBytes = &Metric{
		Measure: stats.Int64(
			"cloudsql.googleapis.com/database/proxy/client/sent_bytes_count",
			"Count of bytes sent.",
			stats.UnitBytes,
		),
		Agg:  view.Sum(),
		Keys: []octag.Key{tag.IPTypeKey},
	}

	// TLSCfgRefreshThrottleCount represents the count of the event that TLS config refreshing
	// throttled.
	TLSCfgRefreshThrottleCount = &Metric{
		Measure: stats.Int64(
			"cloudsql.googleapis.com/database/proxy/client/throttled_TLS_config_refresh_count",
			"Count of times a TLS config refresh was throttled.",
			stats.UnitDimensionless,
		),
		Agg: view.Count(),
	}

	// Version represents the version of proxy client. Note that this metric value is of string
	// type.
	Version = &Metric{
		Measure: stats.Int64(
			"cloudsql.googleapis.com/database/proxy/client/version",
			"The version of the proxy client.",
			stats.UnitDimensionless,
		),
		Agg:  view.LastValue(),
		Keys: []octag.Key{tag.StrValKey},
	}

	// When new metric is defined, add its definition here. And make sure that all configuration
	// is valid.
)

// metricList is the list of metrics. It is used by Initialize() to initialize metrics in it.
var metricList = []*Metric{
	AdminAPIReqCount,
	ActiveConnNum,
	ConnLatency,
	ConnReqCount,
	TermCodeCount,
	SentBytes,
	RcvBytes,
	TLSCfgRefreshThrottleCount,
	Version,
	// When new metric is defined, add the enum const here.
}

// Initialize creates OpenCensus View objects for each of the metrics defined in this package, and
// registers the views. Data from unregistered views are not exported.
func Initialize() error {
	for _, m := range metricList {
		v := &view.View{
			Name:        m.Measure.Name(),
			Description: m.Measure.Description(),
			TagKeys:     append(tag.MandatoryKeys, m.Keys...),
			Measure:     m.Measure,
			Aggregation: m.Agg,
		}
		if err := view.Register(v); err != nil {
			return fmt.Errorf("view.Register() of the metric %v failed: %v", m, err)
		}
	}
	return nil
}

// makeExpBucket makes exponential bucket boundaries. base and scale must be greater than 0 and
// numFinite must be at least 0.
func makeExpBucket(base, scale float64, numFinite int) []float64 {
	ret := make([]float64, 0, numFinite+1)
	for i := 0; i <= numFinite; i++ {
		ret = append(ret, base*math.Pow(scale, float64(i)))
	}
	return ret
}
