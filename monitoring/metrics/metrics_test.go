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

package metrics

import (
	"testing"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
)

// TestMetricDefinition checks definition of all metrics.
func TestMetricDefinition(t *testing.T) {
	if err := Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	for _, m := range metricList {
		for _, key := range m.Keys {
			if key == tag.StrValKey {
				if _, ok := m.Measure.(*stats.Int64Measure); !ok {
					t.Errorf("metric %v is has value type string, measure type: %T, want: %T", m, m.Measure, (*stats.Int64Measure)(nil))
				}
				if aggType := m.Agg.Type; aggType != view.AggTypeLastValue {
					t.Errorf("metric %v is has value type string, aggregation type: %v, want: %v", m, aggType, view.AggTypeLastValue)
				}
			}
		}

		v := view.Find(m.Measure.Name())
		if v == nil {
			t.Errorf("cannot find the view corresponds to the metric: %v", m)
		}
		for _, mandatoryKey := range mandatoryKeys {
			match := false
			for _, viewKey := range v.TagKeys {
				if viewKey == mandatoryKey {
					match = true
				}
			}
			if !match {
				t.Errorf("metric %v does not contain mandatory key: %s", m, mandatoryKey.Name())
			}

		}
	}
}
