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
	"reflect"
	"sort"
	"time"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/metrics"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/util"
	"go.opencensus.io/stats"
	octag "go.opencensus.io/tag"
)

type contextKey struct{}

// ctxKey is used as the key that marks a context is obtained from AddTags.
var ctxKey = contextKey{}

// AddTags returns a context with tags with key ProjectKey, RegionKey, and DatabaseKey. Values of
// project, region and data are obtained from parsing instance. If instance name does not contain
// region, we use "UNSPECIFIED" for it. When setting tags, this function overrides any existing tags
// in ctx. The context returned from this function and contexts propagated from it are suitable to
// use as an argument of recording functions in this package.
func AddTags(ctx context.Context, instance string) (context.Context, error) {
	project, region, database := util.SplitName(instance)
	if region == "" {
		region = "UNSPECIFIED"
	}

	newCtx, err := octag.New(ctx,
		octag.Upsert(tag.ProjectKey, project),
		octag.Upsert(tag.RegionKey, region),
		octag.Upsert(tag.DatabaseKey, database),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update the context with default tags for instance %s: %v", instance, err)
	}

	return context.WithValue(newCtx, ctxKey, true), nil
}

// Remarks on recording functions.
// 1. These functions expects ctx propagated from the one obtained AddTags. If not, all record
//    functions are no-op.
// 2. The metric must have measure and aggregation type that matches the name of the method.
// 3. If there are collision of tag keys, tags in ctx are overridden by tags in argument, and for
//    tags in argument, the latter argument takes precedencs.

// Increment increments the integer counter of the metric.
func Increment(ctx context.Context, m *metrics.Metric, tags ...octag.Tag) error {
	return record(ctx, metrics.ValCount, m, int64(1), tags...)
}

// SetVal records a value, of type int64, on the provided metric.
func SetVal(ctx context.Context, m *metrics.Metric, val int64, tags ...octag.Tag) error {
	return record(ctx, metrics.ValInt, m, val, tags...)
}

// SetDuration records a duration value, expressed as a milliseconds in float64 form, on the
// provided metric.
func SetDuration(ctx context.Context, m *metrics.Metric, val time.Duration, tags ...octag.Tag) error {
	return record(ctx, metrics.ValDuration, m, float64(val/time.Millisecond), tags...)
}

// SetStr records a string value on the provided metric as a value in tag with key tag.StrValKey.
func SetStr(ctx context.Context, m *metrics.Metric, val string, tags ...octag.Tag) error {
	return record(ctx, metrics.ValString, m, val, tags...)
}

// record is the implementation of all recording functions.
func record(ctx context.Context, valType metrics.ValueType, m *metrics.Metric, val interface{}, tags ...octag.Tag) error {
	// If the context did not originated from AddTags, we ignore it.
	if val, ok := ctx.Value(ctxKey).(bool); !ok || !val {
		return nil
	}

	if wantValType := m.ValType(); valType != wantValType {
		return fmt.Errorf("invalid metric value type on metric %s, got: %v, want: %v", m, valType, wantValType)
	}

	var mutator []octag.Mutator
	for _, tg := range tags {
		mutator = append(mutator, octag.Upsert(tg.Key, tg.Value))
	}
	ctx, err := octag.New(ctx, mutator...)
	if err != nil {
		return fmt.Errorf("creating context with input tags failed: %v", err)
	}

	// For string valued metric, add the value to the tag, and set val suitable for dummy int
	// measure of the metric.
	if valType == metrics.ValString {
		strVal := val.(string)
		if ctx, err = octag.New(ctx, octag.Upsert(tag.StrValKey, strVal)); err != nil {
			return fmt.Errorf("setting string value failed: val: %q, err: %v", strVal, err)
		}
		val = int64(0)
	}

	// Check the list of tag keys by comparing keys in ctx and expected keys of the metric.
	// We pass a slice constructor to opencensus' decoder to get the tag keys list in the ctx.
	var gotKeys []string
	makeGotKeys := func(k octag.Key, _ string) {
		gotKeys = append(gotKeys, k.Name())
	}
	err = octag.DecodeEach(octag.Encode(octag.FromContext(ctx)), makeGotKeys)
	if err != nil {
		return fmt.Errorf("failed to get the list of tag keys from the context: %v", err)
	}

	// The metric's key list include mandatory keys for all metrics.
	var wantKeys []string
	for _, k := range append(tag.MandatoryKeys, m.Keys...) {
		wantKeys = append(wantKeys, k.Name())
	}

	sort.Strings(gotKeys)
	sort.Strings(wantKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		return fmt.Errorf("invalid tag keys list for metric %v: tag keys got from context and arguments: %v, want: %v", m, gotKeys, wantKeys)
	}

	var measurement stats.Measurement
	switch measure := m.Measure.(type) {
	case *stats.Int64Measure:
		measurement = measure.M(val.(int64))
	case *stats.Float64Measure:
		measurement = measure.M(val.(float64))
	default:
		return fmt.Errorf("unknown measure type: %T", measure)
	}

	recordFunc(ctx, measurement)
	return nil
}

// recordFunc is a wrapper of stats.Record and will be mocked by tests.
var recordFunc = stats.Record
