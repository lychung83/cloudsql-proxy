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

// Package record provides monitored metric recording facilities.
package record

import (
	"context"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/metrics"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/util"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	octag "go.opencensus.io/tag"
)

type contextKey struct{}

// ctxKey is used as the key that marks a context is obtained from GetContext.
var ctxKey = contextKey{}

// GetContext returns a context with tags with key ProjectKey, RegionKey, DatabaseKey, IPTypeKey.
// Values project, region and data are obtained from parsing instance, and value for IP type is set
// to "UNKNOWN". If instance name does not contain region, we use "UNSPECIFIED" for it. When setting
// tags, this function overrides any existing tags in ctx. The context returned from this function
// and contexts propagated from it are suitable to use as an argument of recording functions in this
// package.
func GetContext(ctx context.Context, instance string) (context.Context, error) {
	project, region, database := util.SplitName(instance)
	if region == "" {
		region = "UNSPECIFIED"
	}

	newCtx, err := octag.New(ctx,
		octag.Upsert(tag.ProjectKey, project),
		octag.Upsert(tag.RegionKey, region),
		octag.Upsert(tag.DatabaseKey, database),
		octag.Upsert(tag.IPTypeKey, "UNKNOWN"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update the context with default tags for instance monitoring: %v", err)
	}

	// We mark the context with key-value (ctxKey: true) to indicate that the context is
	// obtained from this function.
	return context.WithValue(newCtx, ctxKey, true), nil
}

// Remarks on Record*() functions.
// 1. These functions expects ctx propagated from the one obtained GetContext. If not, all record
//    functions are no-op.
// 2. The metric must have measure and aggregation type that matches the name of the method.
// 3. tags (including tags defined in ctx) must correspond to tags expected by the metric with
//    following exceptions.
//    3.1. Even when metric does not expect IPTypeKey, tags may contain IPTypeKey.
//    3.2. String valued metric needs no tag with StrValKey. Str() will set it, and replace any
//         existing value in tag with StrValKey.
// 4. If there are collision of tag keys, tags in ctx are overridden by tags in argument, and for
//    tags in argument, the latter argument takes precedencs.

// Count increments the integer counter of the metric.
func Count(ctx context.Context, m *metrics.Metric, tags ...octag.Tag) error {
	// RecordCount itself does not require value, but increase the value by 1 internally.
	return record(ctx, valCount, m, int64(1), tags...)
}

// Int records an int64 of the metric.
func Int(ctx context.Context, m *metrics.Metric, val int64, tags ...octag.Tag) error {
	return record(ctx, valInt, m, val, tags...)
}

// Duration records a duration value of the metric in float64 form with millisecond unit.
func Duration(ctx context.Context, m *metrics.Metric, val time.Duration, tags ...octag.Tag) error {
	return record(ctx, valDuration, m, float64(val)/float64(time.Millisecond), tags...)
}

// Str records a string value of the metric.
func Str(ctx context.Context, m *metrics.Metric, val string, tags ...octag.Tag) error {
	return record(ctx, valString, m, val, tags...)
}

// valueType represents the value type used for recording.
type valueType int

const (
	valCount valueType = iota
	valInt
	valDuration
	valString
)

// valTypeMatch checks whether valType matches that of the metric.
func valTypeMatch(valType valueType, m *metrics.Metric) bool {
	// String valued metric is identified by the presence of the key StrValKey.
	isValueString := false
	for _, key := range m.Keys {
		if key == tag.StrValKey {
			isValueString = true
			break
		}
	}
	if isValueString {
		return valType == valString
	}

	if m.Agg.Type == view.AggTypeCount {
		return valType == valCount
	}

	switch m.Measure.(type) {
	case *stats.Int64Measure:
		return valType == valInt
	case *stats.Float64Measure:
		return valType == valDuration
	default:
		return false
	}
}

// record is the implementation of all Record* methods.
func record(ctx context.Context, valType valueType, m *metrics.Metric, val interface{}, tags ...octag.Tag) error {
	// If the context did not originated from GetContext, we ignore it.
	if val, ok := ctx.Value(ctxKey).(bool); !(val == true && ok) {
		return nil
	}

	if !valTypeMatch(valType, m) {
		return fmt.Errorf("invalid metric: %v", m)
	}

	useIPType := false
	metricKeys := map[octag.Key]bool{
		tag.ProjectKey:  true,
		tag.RegionKey:   true,
		tag.DatabaseKey: true,
	}
	for _, metricKey := range m.Keys {
		metricKeys[metricKey] = true
		if metricKey == tag.IPTypeKey {
			useIPType = true
		}
	}

	// We create context containing all tags.
	var mutator []octag.Mutator
	for _, tg := range tags {
		mutator = append(mutator, octag.Upsert(tg.Key, tg.Value))
	}
	ctx, err := octag.New(ctx, mutator...)
	if err != nil {
		return fmt.Errorf("creating context with input tags failed: %v", err)
	}
	if !useIPType {
		if ctx, err = octag.New(ctx, octag.Delete(tag.IPTypeKey)); err != nil {
			return fmt.Errorf("deleting unwanted IP type key failed: %v", err)
		}
	}
	if valType == valString {
		strVal := val.(string)
		if ctx, err = octag.New(ctx, octag.Upsert(tag.StrValKey, strVal)); err != nil {
			return fmt.Errorf("setting string value failed: val: %q, err: %v", strVal, err)
		}
		// The string value is recorded into tag. Now we set val suitable for dummy int
		// measure of the metric.
		val = int64(0)
	}

	// Check the list of tags.
	inputTags := map[octag.Key]string{}
	makeInputTags := func(key octag.Key, val string) {
		inputTags[key] = val
	}
	err = octag.DecodeEach(octag.Encode(octag.FromContext(ctx)), makeInputTags)
	if err != nil {
		return fmt.Errorf("failed to get the list of tag keys from the context: %v", err)
	}

	for inputKey, inputValue := range inputTags {
		if !metricKeys[inputKey] {
			return fmt.Errorf("invalid tag for the metric provided: key: %s, value: %s", inputKey.Name(), inputValue)
		}
	}

	for metricKey := range metricKeys {
		if _, ok := inputTags[metricKey]; !ok {
			return fmt.Errorf("metric expects a tag with following key, but not provided: %s", metricKey.Name())
		}
	}

	// Get the measurement from the val.
	var measurement stats.Measurement
	switch measure := m.Measure.(type) {
	case *stats.Int64Measure:
		measurement = measure.M(val.(int64))
	case *stats.Float64Measure:
		measurement = measure.M(val.(float64))
	default:
		return fmt.Errorf("unknown measure type: %T", measure)
	}

	// We have both tags and values suitable for recording, so proceed.
	recordFunc(ctx, measurement)
	return nil
}

// recordFunc is a wrapper of stats.Record and will be mocked by tests.
var recordFunc = stats.Record
