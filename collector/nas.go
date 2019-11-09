package collector

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/aokumasan/nifcloud-sdk-go-v2/nifcloud"
	"github.com/aokumasan/nifcloud-sdk-go-v2/service/nas"
	"github.com/aws/aws-sdk-go-v2/private/protocol/query/queryutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

const (
	namespace       = "nifcloud_nas"
	timestampLayout = "2006-01-02 15:04:05"
)

var (
	scrapeDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "scrape", "collector_duration_seconds"),
		"nifcloud_nas_exporter: Duration of a collector scrape.",
		[]string{"metric_name"},
		nil,
	)
	scrapeSuccessDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "scrape", "collector_success"),
		"nifcloud_nas_exporter: Whether a collector succeeded.",
		[]string{"metric_name"},
		nil,
	)

	label = []string{"instance", "region"}

	metrics = []Metric{
		{
			Name: "FreeStorageSpace",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "free_storage_space"),
				"The amount of available storage space. Units: Bytes",
				label, nil,
			),
		},
		{
			Name: "UsedStorageSpace",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "used_storage_space"),
				"The amount of used storage space. Units: Bytes",
				label, nil,
			),
		},
		{
			Name: "ReadIOPS",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "read_iops"),
				"The average number of disk read I/O operations per second. Units: Count/Second",
				label, nil,
			),
		},
		{
			Name: "WriteIOPS",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "write_iops"),
				"The average number of disk write I/O operations per second. Units: Count/Second",
				label, nil,
			),
		},
		{
			Name: "ReadThroughput",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "read_throughput"),
				"The average number of bytes read from disk per second. Units: Bytes/Second",
				label, nil,
			),
		},
		{
			Name: "WriteThroughput",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "write_throughput"),
				"The average number of bytes written to disk per second. Units: Bytes/Second",
				label, nil,
			),
		},
		{
			Name: "ActiveConnections",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "active_connections"),
				"The active connection counts. Units: Count",
				label, nil,
			),
		},
		{
			Name: "GlobalReadTraffic",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "global_read_traffic"),
				"The incoming (Receive) network traffic from global on the NAS instance. Units: Bytes/second",
				label, nil,
			),
		},
		{
			Name: "PrivateReadTraffic",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "private_read_traffic"),
				"The incoming (Receive) network traffic from private on the NAS instance. Units: Bytes/second",
				label, nil,
			),
		},
		{
			Name: "GlobalWriteTraffic",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "global_write_traffic"),
				"The outgoing (Transmit) network traffic to global on the NAS instance. Units: Bytes/second",
				label, nil,
			),
		},
		{
			Name: "PrivateWriteTraffic",
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "", "private_write_traffic"),
				"The outgoing (Transmit) network traffic to private on the NAS instance. Units: Bytes/second",
				label, nil,
			),
		},
	}
)

type Metric struct {
	Name string
	Desc *prometheus.Desc
}

type NASCollector struct {
	client                *nas.Client
	metrics               []Metric
	nasInstanceIdentifier string
	region                string
}

func NewNASCollector(nasInstanceIdentifier, accessKeyID, secretAccessKey, region string) *NASCollector {
	return &NASCollector{
		client:                nas.New(nifcloud.NewConfig(accessKeyID, secretAccessKey, region)),
		metrics:               metrics,
		nasInstanceIdentifier: nasInstanceIdentifier,
		region:                region,
	}
}

func (n NASCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range n.metrics {
		ch <- m.Desc
	}

	ch <- scrapeDurationDesc
	ch <- scrapeSuccessDesc
}

func (n NASCollector) Collect(ch chan<- prometheus.Metric) {
	wg := sync.WaitGroup{}
	wg.Add(len(n.metrics))
	for _, m := range n.metrics {
		go func(metric Metric) {
			n.collect(metric, ch)
			wg.Done()
		}(m)
	}
	wg.Wait()
}

func (n NASCollector) collect(metric Metric, ch chan<- prometheus.Metric) {
	begin := time.Now()
	err := n.scrape(metric, ch)
	duration := time.Since(begin)
	var success float64

	if err != nil {
		log.Errorf("ERROR: scrape %q failed after %fs: %v", metric.Name, duration, err)
		success = 0
	} else {
		success = 1
	}

	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration.Seconds(), metric.Name)
	ch <- prometheus.MustNewConstMetric(scrapeSuccessDesc, prometheus.GaugeValue, success, metric.Name)
}

func (n NASCollector) scrape(metric Metric, ch chan<- prometheus.Metric) error {
	now := time.Now().In(time.UTC)
	request := n.client.GetMetricStatisticsRequest(&nas.GetMetricStatisticsInput{
		Dimensions: []nas.RequestDimensionsStruct{
			{
				Name:  nifcloud.String("NASInstanceIdentifier"),
				Value: nifcloud.String(n.nasInstanceIdentifier),
			},
		},
		MetricName: nifcloud.String(metric.Name),
	})
	if err := request.Build(); err != nil {
		return fmt.Errorf("failed building request: %v", err)
	}
	body := url.Values{
		"Action":  {request.Operation.Name},
		"Version": {request.Metadata.APIVersion},
	}
	if err := queryutil.Parse(body, request.Params, false); err != nil {
		return fmt.Errorf("failed encoding request: %v", err)
	}
	body.Set("StartTime", now.Add(time.Duration(180)*time.Second*-1).Format(timestampLayout)) // 3 min (to fetch at least 1 data-point)
	body.Set("EndTime", now.Format(timestampLayout))
	request.SetBufferBody([]byte(body.Encode()))

	response, err := request.Send(context.Background())
	if err != nil {
		return err
	}

	datapoints := response.Datapoints
	if len(datapoints) == 0 {
		return errors.New("fetched no datapoints")
	}

	latest := new(time.Time)
	var latestVal float64
	for _, dp := range datapoints {
		timestamp, err := time.Parse(time.RFC3339, nifcloud.StringValue(dp.Timestamp))
		if err != nil {
			return fmt.Errorf("could not parse timestamp %q: %v", nifcloud.StringValue(dp.Timestamp), err)
		}

		if timestamp.Before(*latest) {
			continue
		}

		latest = &timestamp
		latestVal, err = strconv.ParseFloat(nifcloud.StringValue(dp.Sum), 64)
		if err != nil {
			return fmt.Errorf("could not parse sum %q: %v", nifcloud.StringValue(dp.Sum), err)
		}
	}

	ch <- prometheus.MustNewConstMetric(metric.Desc, prometheus.GaugeValue, latestVal, n.nasInstanceIdentifier, n.region)

	return nil
}
