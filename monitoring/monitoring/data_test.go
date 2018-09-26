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

package monitoring

import (
	"errors"
	"time"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"

	"go.opencensus.io/stats/view"
	octag "go.opencensus.io/tag"
)

// This file defines data used in tests.

const (
	errMsg1 = "error message 1"
	errMsg2 = "error message 2"

	metric1Name = "cloudsql.googleapis.com/database/proxy/client/metric_1"
	metric2Name = "cloudsql.googleapis.com/database/proxy/client/metric_2"
	// metric3Name is invalid metric name
	metric3Name = "metric_3"
	project1    = "project-1"
	project2    = "project-2"
	region1     = "us-central1"
	region2     = "us-east1"
	database1   = "instance-1"
	database2   = "instance-2"

	uuidVal = "9767319e-7726-42d8-9e28-5c8ddb2d54c2"
)

var (
	start    = now.Add(-time.Minute)
	startStr = start.Format(time.RFC3339)
	now      = time.Now()
	nowStr   = now.Format(time.RFC3339)

	err1 = errors.New(errMsg1)
	err2 = errors.New(errMsg2)

	tags1 = []octag.Tag{{tag.ProjectKey, project1}, {tag.RegionKey, region1}, {tag.DatabaseKey, database1}}
	tags2 = []octag.Tag{{tag.ProjectKey, project2}, {tag.RegionKey, region2}, {tag.DatabaseKey, database2}}
	// tags3 is missing projectKey.
	tags3 = []octag.Tag{{tag.RegionKey, region1}, {tag.DatabaseKey, database1}}
	view1 = &view.View{Name: metric1Name}
	view2 = &view.View{Name: metric2Name}
	row1  = &view.Row{Tags: tags1}
	row2  = &view.Row{Tags: tags2}
)

func init() {
	// We manually set uuid.
	uuid = uuidVal
}
