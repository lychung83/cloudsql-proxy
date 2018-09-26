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
	"fmt"
	"os"
	"strings"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	mpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

var warningMsg = `
*********************************************************************************
* DEBUG LOG: THIS PROGRAM IS USING FAKE MONITORING API FOR DEVELOPMENT PURPOSE. *
*********************************************************************************
`[1:]

// WARNING: this file contains development purpose fake version of monitoring API. This code should
// not be used in any production environment at all.

func init() {
	if os.Getenv("CLOUD_SQL_PROXY_DEV") != "" {
		fmt.Printf(warningMsg)
		createTimeSeries = fakeCreateTimeSeries
	}
}

func fakeCreateTimeSeries(ctx context.Context, cl *monitoring.MetricClient, req *mpb.CreateTimeSeriesRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}

	var fakeTsArr []*mpb.TimeSeries
	for i, ts := range req.TimeSeries {
		errPrefix := fmt.Sprintf("timeseries[%d]", i)
		name := ts.Metric.Type
		sepIndex := strings.IndexByte(name, '/')
		if sepIndex == -1 {
			return fmt.Errorf("%s: metric name does not have '/': %s", errPrefix, name)
		}
		fakeName := fmt.Sprintf("custom.googleapis.com/cloudsql%s", name[sepIndex:])

		fakeRscLabels := make(map[string]string)
		var uuid string
		for key, val := range ts.Resource.Labels {
			var fakeKey string
			fakeVal := val
			switch key {
			case "uuid":
				uuid = val
				continue
			case "project_id":
				fakeKey = key
			case "region":
				fakeKey = "zone"
				if val == "UNSPECIFIED" {
					fakeVal = "global"
				}
			case "database_id":
				fakeKey = "instance_id"
			default:
				return fmt.Errorf("unknown label key of monitored resource: %s", key)
			}
			fakeRscLabels[fakeKey] = fakeVal
		}
		fakeResource := &mrpb.MonitoredResource{
			Type:   "gce_instance",
			Labels: fakeRscLabels,
		}

		isValueString := false
		var strVal string

		fakeVal := ts.Points[0].Value.Value
		if strTypeVal, ok := fakeVal.(*mpb.TypedValue_StringValue); ok {
			isValueString = true
			strVal = strTypeVal.StringValue
			fakeVal = &mpb.TypedValue_Int64Value{
				Int64Value: 0,
			}
		}
		fakePt := &mpb.Point{
			Interval: ts.Points[0].Interval,
			Value: &mpb.TypedValue{
				Value: fakeVal,
			},
		}

		fakeLabels := map[string]string{"uuid": uuid}
		for key, val := range ts.Metric.Labels {
			fakeLabels[key] = val
		}
		if isValueString {
			fakeLabels["string_metric_value"] = strVal
		}

		fakeTs := &mpb.TimeSeries{
			Metric: &metricpb.Metric{
				Type:   fakeName,
				Labels: fakeLabels,
			},
			Resource: fakeResource,
			Points:   []*mpb.Point{fakePt},
		}
		fakeTsArr = append(fakeTsArr, fakeTs)
	}

	fakeReq := &mpb.CreateTimeSeriesRequest{
		Name:       req.Name,
		TimeSeries: fakeTsArr,
	}
	return cl.CreateTimeSeries(ctx, fakeReq)
}
