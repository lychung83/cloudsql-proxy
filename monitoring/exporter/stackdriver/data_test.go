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

package stackdriver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

// This file defines various data needed for testing.

const (
	label1name = "key_1"
	label2name = "key_2"
	label3name = "key_3"
	label4name = "key_4"
	label5name = "key_5"

	value1 = "value_1"
	value2 = "value_2"
	value3 = "value_3"
	value4 = "value_4"
	value5 = "value_5"
	value6 = "value_6"
	value7 = "value_7"
	value8 = "value_8"

	metric1name = "metric_1"
	metric1desc = "this is metric 1"
	metric2name = "metric_2"
	metric2desc = "this is metric 2"
	metric3name = "metric_3"
	metric3desc = "this is metric 3"

	project1 = "project-1"
	project2 = "project-2"
)

var (
	ctx = context.Background()

	invalidDataErrStr = "invalid data"
	// This error is used for test to catch some error happpened.
	invalidDataError = errors.New(invalidDataErrStr)
	// This error is used for unexpected error.
	unrecognizedDataError = errors.New("unrecognized data")

	key1       = getKey(label1name)
	key2       = getKey(label2name)
	key3       = getKey(label3name)
	projectKey = getKey(ProjectKeyName)

	view1 = &view.View{
		Name:        metric1name,
		Description: metric1desc,
		TagKeys:     nil,
		Measure:     stats.Int64(metric1name, metric1desc, stats.UnitDimensionless),
		Aggregation: view.Sum(),
	}
	view2 = &view.View{
		Name:        metric2name,
		Description: metric2desc,
		TagKeys:     []tag.Key{key1, key2, key3},
		Measure:     stats.Int64(metric2name, metric2desc, stats.UnitDimensionless),
		Aggregation: view.Sum(),
	}
	view3 = &view.View{
		Name:        metric3name,
		Description: metric3desc,
		TagKeys:     []tag.Key{projectKey},
		Measure:     stats.Int64(metric3name, metric3desc, stats.UnitDimensionless),
		Aggregation: view.Sum(),
	}

	// To make verification easy, we require all valid rows should have int64 values and all of
	// them must be distinct.
	view1row1 = &view.Row{
		Tags: nil,
		Data: &view.SumData{Value: 1},
	}
	view1row2 = &view.Row{
		Tags: nil,
		Data: &view.SumData{Value: 2},
	}
	view1row3 = &view.Row{
		Tags: nil,
		Data: &view.SumData{Value: 3},
	}
	view2row1 = &view.Row{
		Tags: []tag.Tag{{key1, value1}, {key2, value2}, {key3, value3}},
		Data: &view.SumData{Value: 4},
	}
	view2row2 = &view.Row{
		Tags: []tag.Tag{{key1, value4}, {key2, value5}, {key3, value6}},
		Data: &view.SumData{Value: 5},
	}
	view3row1 = &view.Row{
		Tags: []tag.Tag{{projectKey, project1}},
		Data: &view.SumData{Value: 6},
	}
	view3row2 = &view.Row{
		Tags: []tag.Tag{{projectKey, project2}},
		Data: &view.SumData{Value: 7},
	}
	view3row3 = &view.Row{
		Tags: []tag.Tag{{projectKey, project1}},
		Data: &view.SumData{Value: 8},
	}
	// This Row does not have valid Data field, so is invalid.
	invalidRow = &view.Row{Data: nil}

	resource1 = &mrpb.MonitoredResource{
		Type: "cloudsql_database",
		Labels: map[string]string{
			"project_id":  project1,
			"region":      "us-central1",
			"database_id": "cloud-SQL-instance-1",
		},
	}
	resource2 = &mrpb.MonitoredResource{
		Type: "gce_instance",
		Labels: map[string]string{
			"project_id":  project2,
			"zone":        "us-east1",
			"database_id": "GCE-instance-1",
		},
	}
)

// Timestamps. We make sure that all time stamps are strictly increasing.
var (
	startTime1, endTime1, startTime2, endTime2 time.Time
	startTime3, endTime3, startTime4, endTime4 time.Time
)

func init() {
	ts := time.Now()
	for _, t := range []*time.Time{
		&startTime1, &endTime1, &startTime2, &endTime2,
		&startTime3, &endTime3, &startTime4, &endTime4,
	} {
		*t = ts
		ts = ts.Add(time.Second)
	}
}

func getKey(name string) tag.Key {
	key, err := tag.NewKey(name)
	if err != nil {
		panic(fmt.Errorf("key creation failed for key name: %s", name))
	}
	return key
}
