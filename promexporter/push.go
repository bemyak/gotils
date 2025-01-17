// Copyright 2021 RELEX Oy
// Copyright 2014 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promexporter

import (
	"bytes"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/relex/gotils/logger"
)

const pushMetricsTimeout = 20 * time.Second

// DumpMetricsForTest dumps metrics from default registry in the .prom text format without comments
//
// For testing only
func DumpMetricsForTest(prefix string, skipZeroValues bool) string {
	return DumpMetricsFrom(prometheus.DefaultGatherer, prefix, true, skipZeroValues)
}

// DumpMetricsFrom dumps metrics from the given gatherer in the .prom text
func DumpMetricsFrom(gatherer prometheus.Gatherer, prefix string, skipComments, skipZeroValues bool) string {
	metricFamilies, err := gatherer.Gather()
	if err != nil {
		logger.Panicf("failed to gather metrics: %v", err)
	}
	writer := &bytes.Buffer{}
	for _, mf := range metricFamilies {
		if !strings.HasPrefix(mf.GetName(), prefix) {
			continue
		}
		if _, err := expfmt.MetricFamilyToText(writer, mf); err != nil {
			logger.Panicf("failed to export '%s': %v", *mf.Name, err)
		}
	}
	lines := strings.Split(writer.String(), "\n")
	linesFiltered := make([]string, 0, len(lines)/2)
	for _, ln := range lines {
		if skipComments && strings.HasPrefix(ln, "#") {
			continue
		}
		if skipZeroValues && strings.HasSuffix(ln, " 0") {
			continue
		}
		linesFiltered = append(linesFiltered, ln)
	}
	return strings.Join(linesFiltered, "\n")
}

// PushMetrics pushes all metrics in the default registry to the target URL
//
// The URL should contain no path for the official pushgateway
func PushMetrics(url string, job string) {
	client := &http.Client{}
	client.Timeout = pushMetricsTimeout // default is no timeout
	err := push.New(url, job).Gatherer(prometheus.DefaultGatherer).Client(client).Push()
	if err != nil {
		logger.Error("failed to push metrics: ", err)
	}
}

// SumMetricValues sums all the values of a given Prometheus Collector (GaugeVec or CounterVec)
//
// Only works with top-level MetricVec, not curried MetricVec
func SumMetricValues(c prometheus.Collector) float64 {
	// modified from github.com/prometheus/client_golang/prometheus/testutil.ToFloat64
	var (
		mList = make([]prometheus.Metric, 0, 100)
		mChan = make(chan prometheus.Metric)
		done  = make(chan struct{})
	)
	go func() {
		for m := range mChan {
			mList = append(mList, m)
		}
		close(done)
	}()
	c.Collect(mChan)
	close(mChan)
	<-done

	sum := 0.0
	for _, m := range mList {
		pb := &dto.Metric{}
		if err := m.Write(pb); err != nil {
			// don't return error as it should be impossible
			logger.Errorf("failed to read metric '%s': %s", m.Desc(), err.Error())
		}
		if pb.Gauge != nil {
			sum += pb.Gauge.GetValue()
		}
		if pb.Counter != nil {
			sum += pb.Counter.GetValue()
		}
		if pb.Untyped != nil {
			sum += pb.Untyped.GetValue()
		}

	}
	return sum
}
