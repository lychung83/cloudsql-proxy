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
	"time"

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	dspb "google.golang.org/genproto/googleapis/api/distribution"
	mpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

// Functions in this file is used to convert RowData to monitoring point that are used by uploading
// monitoring API calls. All functions except newStringPoint in this file are copied from
// contrib.go.opencensus.io/exporter/stackdriver.

func newPoint(v *view.View, row *view.Row, start, end time.Time) *mpb.Point {
	switch v.Aggregation.Type {
	case view.AggTypeLastValue:
		return newGaugePoint(v, row, end)
	default:
		return newCumulativePoint(v, row, start, end)
	}
}

// newStringPoint returns a metric point with string value.
func newStringPoint(val string, end time.Time) *mpb.Point {
	gaugeTime := &tspb.Timestamp{
		Seconds: end.Unix(),
		Nanos:   int32(end.Nanosecond()),
	}
	return &mpb.Point{
		Interval: &mpb.TimeInterval{
			EndTime: gaugeTime,
		},
		Value: &mpb.TypedValue{
			Value: &mpb.TypedValue_StringValue{
				StringValue: val,
			},
		},
	}
}

func newCumulativePoint(v *view.View, row *view.Row, start, end time.Time) *mpb.Point {
	return &mpb.Point{
		Interval: &mpb.TimeInterval{
			StartTime: &tspb.Timestamp{
				Seconds: start.Unix(),
				Nanos:   int32(start.Nanosecond()),
			},
			EndTime: &tspb.Timestamp{
				Seconds: end.Unix(),
				Nanos:   int32(end.Nanosecond()),
			},
		},
		Value: newTypedValue(v, row),
	}
}

func newGaugePoint(v *view.View, row *view.Row, end time.Time) *mpb.Point {
	gaugeTime := &tspb.Timestamp{
		Seconds: end.Unix(),
		Nanos:   int32(end.Nanosecond()),
	}
	return &mpb.Point{
		Interval: &mpb.TimeInterval{
			EndTime: gaugeTime,
		},
		Value: newTypedValue(v, row),
	}
}

func newTypedValue(vd *view.View, r *view.Row) *mpb.TypedValue {
	switch v := r.Data.(type) {
	case *view.CountData:
		return &mpb.TypedValue{Value: &mpb.TypedValue_Int64Value{
			Int64Value: v.Value,
		}}
	case *view.SumData:
		switch vd.Measure.(type) {
		case *stats.Int64Measure:
			return &mpb.TypedValue{Value: &mpb.TypedValue_Int64Value{
				Int64Value: int64(v.Value),
			}}
		case *stats.Float64Measure:
			return &mpb.TypedValue{Value: &mpb.TypedValue_DoubleValue{
				DoubleValue: v.Value,
			}}
		}
	case *view.DistributionData:
		return &mpb.TypedValue{Value: &mpb.TypedValue_DistributionValue{
			DistributionValue: &dspb.Distribution{
				Count:                 v.Count,
				Mean:                  v.Mean,
				SumOfSquaredDeviation: v.SumOfSquaredDev,
				// TODO(songya): uncomment this once Stackdriver supports min/max.
				// Range: &dspb.Distribution_Range{
				//      Min: v.Min,
				//      Max: v.Max,
				// },
				BucketOptions: &dspb.Distribution_BucketOptions{
					Options: &dspb.Distribution_BucketOptions_ExplicitBuckets{
						ExplicitBuckets: &dspb.Distribution_BucketOptions_Explicit{
							Bounds: vd.Aggregation.Buckets,
						},
					},
				},
				BucketCounts: v.CountPerBucket,
			},
		}}
	case *view.LastValueData:
		switch vd.Measure.(type) {
		case *stats.Int64Measure:
			return &mpb.TypedValue{Value: &mpb.TypedValue_Int64Value{
				Int64Value: int64(v.Value),
			}}
		case *stats.Float64Measure:
			return &mpb.TypedValue{Value: &mpb.TypedValue_DoubleValue{
				DoubleValue: v.Value,
			}}
		}
	}
	return nil
}
