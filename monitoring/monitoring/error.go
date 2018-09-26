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

// This file contains facilities for periodic report of errors generated from stackdriver exporter.
// We generally use map[string]int to represent multi-set of strings.

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/logging"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/exporter/stackdriver"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"
)

// errorHandler contains all configuration needed for reporting aggregated errors. errorHandler
// should be created by newErrorHandler(), activated by run(), and finalized by stop().
type errorHandler struct {
	// infoCh is the channel of communication between error handler and stackdriver exporter.
	infoCh chan *errInfo
	// infoWg keeps the number of errInfo waiting to be sent to infoCh.
	infoWg sync.WaitGroup
	// done is used by stop() to notify errorHandler to stop.
	done chan struct{}
	// quit is used by run() to notify that errorHandler stopped.
	quit chan struct{}
	// delay is the delay between error reports.
	delay time.Duration
	// ticker tells error handler to report errors.
	ticker *time.Ticker
}

// newErrorHandler creates an error handler.
func newErrorHandler(delay time.Duration) *errorHandler {
	if delay <= 0 {
		delay = DefaultErrLogDelay
	}
	// We don't initialize ticker yet. It is initialized when error handler starts running.
	return &errorHandler{
		infoCh: make(chan *errInfo),
		done:   make(chan struct{}),
		quit:   make(chan struct{}),
		delay:  delay,
	}
}

// run starts operation of error handler. It blocks the caller indefinitely until stop() is
// returned.
func (eh *errorHandler) run() {
	// Create the firstor error log data and start ticker.
	data := newErrLogData(time.Now())
	eh.ticker = time.NewTicker(eh.delay)
	for {
		select {
		case info := <-eh.infoCh:
			// We received an error. Just add its content to data.
			data.add(info)
		case <-eh.ticker.C:
			// ticker fired. We report the error.
			now := time.Now()
			data.report(now)
			// Flush the data for next error log.
			data = newErrLogData(now)
		case <-eh.done:
			eh.ticker.Stop()
			// Collect all reamrining errors.
			go func() {
				eh.infoWg.Wait()
				close(eh.infoCh)
			}()
			for info := range eh.infoCh {
				data.add(info)
			}
			// Do the last report before finishing.
			data.report(time.Now())
			close(eh.quit)
			return
		}
	}
}

// stop quits the error handler. After stop() is called, no error is reported.
func (eh *errorHandler) stop() {
	close(eh.done)
	<-eh.quit
}

// expHrrHandler is used by stackdriver exporter to pass error to error handler. This function
// constructs an errInfo object from the reported error and pass it to infoCh.
func (eh *errorHandler) expErrHandler(err error, rds ...*stackdriver.RowData) {
	instances := make(map[string]int)
	metrics := make(map[string]int)
	for _, rd := range rds {
		// We get the name of instance by looking at tags.
		unknown := "<unknown>"
		project := unknown
		region := unknown
		database := unknown
		for _, tg := range rd.Row.Tags {
			switch tg.Key {
			case tag.ProjectKey:
				project = tg.Value
			case tag.RegionKey:
				region = tg.Value
			case tag.DatabaseKey:
				database = tg.Value
			}
		}
		instance := fmt.Sprintf("%s:%s:%s", project, region, database)
		// Add instance and metric names.
		instances[instance]++
		metrics[rd.View.Name]++
	}

	// Since expErrHandler() is used by stackdriver exporter and should be non-blocking, we make
	// a separate routine to send data to infoCh.
	info := &errInfo{err.Error(), len(rds), instances, metrics}
	eh.infoWg.Add(1)
	go func() {
		eh.infoCh <- info
		eh.infoWg.Done()
	}()
}

// errInfo is the unit of error information sent from stackdriver exporter to error handler for
// a single error on exporting monitored metric.
type errInfo struct {
	// errMsg is the error message exporter made. Since errInfo contains a single stackdriver
	// exporter error, there is only one error message.
	errMsg string
	// rowNum counts number of rows failed to be published due to the error.
	rowNum int
	// instances stores instance names that failed to be exported due to the error.
	instances map[string]int
	// metrics stores metric names that failed to be exported due to the error.
	metrics map[string]int
}

// errLogData is the collection of error data for a error log cycle.
type errLogData struct {
	// start is the time that this log started to collect data.
	start time.Time
	// errNum is the number of error happened in this error log cycle.
	errNum int
	// errMsgs stores list of error messages collected in this error log cycle.
	errMsgs map[string]int
	// rowNum, instances and metrics have same meanings as those of errInfo.
	rowNum    int
	instances map[string]int
	metrics   map[string]int
}

// newErrLogData creates an errLogData used for one error log cycle.
func newErrLogData(start time.Time) *errLogData {
	return &errLogData{
		start:     start,
		errMsgs:   make(map[string]int),
		instances: make(map[string]int),
		metrics:   make(map[string]int),
	}
}

// add adds information on ino to data.
func (data *errLogData) add(info *errInfo) {
	data.errNum++
	data.rowNum += info.rowNum
	data.errMsgs[info.errMsg]++
	for instance, count := range info.instances {
		data.instances[instance] += count
	}
	for metric, count := range info.metrics {
		data.metrics[metric] += count
	}
}

// report reports collected error from data. If there is no error, then nothing is reported.
// Format looks like follows. We use header and mark beginning and end of the report because error
// report can be intervened by other error logging activity. We also sort error message, instances,
// and names in the report.
//
// stackdriver monitoring error: beginning of the report
// stackdriver monitoring error: span of the report: from 2018-09-05T09:57:00-07:00 to 2018-09-05T09:57:10-07:00
// stackdriver monitoring error: 3 errors are reported and export of 5 data points failed to stackdriver
// stackdriver monitoring error: error messages are:
// stackdriver monitoring error:     (repeated 2 times) error message 1
// stackdriver monitoring error:     error message 2
// stackdriver monitoring error: affected instances are:
// stackdriver monitoring error:     project-1:us-central1:instance-1: export of 1 data points failed in this instance
// stackdriver monitoring error:     project-2:us-east1:instance-2: export of 4 data points failed in this instance
// stackdriver monitoring error: affected metrics are:
// stackdriver monitoring error:     metric_1: export of 3 data points failed in this metric
// stackdriver monitoring error:     metric_2: export of 2 data points failed in this metric
// stackdriver monitoring error: end of the report
func (data *errLogData) report(now time.Time) {
	if data.errNum == 0 {
		return
	}
	prefix := "stackdriver monitoring error:"
	indentPrefix := fmt.Sprintf("%s    ", prefix)

	logging.Errorf("%s beginning of the report", prefix)
	logging.Errorf("%s span of the report: from %s to %s", prefix, data.start.Format(time.RFC3339), now.Format(time.RFC3339))
	logging.Errorf("%s %d errors are reported and export of %d data points failed to stackdriver", prefix, data.errNum, data.rowNum)

	logging.Errorf("%s error messages are:", prefix)
	for _, msg := range sortedKeys(data.errMsgs) {
		if count := data.errMsgs[msg]; count != 1 {
			msg = fmt.Sprintf("(repeated %d times) %s", count, msg)
		}
		logging.Errorf("%s %s", indentPrefix, msg)
	}

	logging.Errorf("%s affected instances are:", prefix)
	for _, instance := range sortedKeys(data.instances) {
		logging.Errorf("%s %s: export of %d data points failed in this instance", indentPrefix, instance, data.instances[instance])
	}

	logging.Errorf("%s affected metrics are:", prefix)
	for _, metric := range sortedKeys(data.metrics) {
		logging.Errorf("%s %s: export of %d data points failed in this metric", indentPrefix, metric, data.metrics[metric])
	}

	logging.Errorf("%s end of the report", prefix)
}

// sortedKeys sorts keys in map[string]int .
func sortedKeys(m map[string]int) []string {
	ret := make([]string, 0, len(m))
	for key := range m {
		ret = append(ret, key)
	}
	sort.StringSlice(ret).Sort()
	return ret
}
