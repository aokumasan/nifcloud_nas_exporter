package main

import (
	"net/http"

	"github.com/aokumasan/nifcloud_nas_exporter/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

const exporterName = "nifcloud_nas_exporter"

func main() {
	var (
		listenAddress = kingpin.Flag(
			"web.listen-address",
			"Address on which to expose metrics and web interface.",
		).Default(":9123").String()
		metricsPath = kingpin.Flag(
			"web.telemetry-path",
			"Path under which to expose metrics.",
		).Default("/metrics").String()
		disableExporterMetrics = kingpin.Flag(
			"web.disable-exporter-metrics",
			"Exclude metrics about the exporter itself (promhttp_*, process_*, go_*).",
		).Bool()
		maxRequests = kingpin.Flag(
			"web.max-requests",
			"Maximum number of parallel scrape requests. Use 0 to disable.",
		).Default("40").Int()
		nasInstanceID = kingpin.Flag(
			"nifcloud.nas-instance-id",
			"Target NAS instance identifier.",
		).Required().String()
		region = kingpin.Flag(
			"nifcloud.region",
			"NIFCLOUD region name that target instance exists.",
		).Default("jp-east-1").String()
		accessKeyID = kingpin.Flag(
			"nifcloud.access-key-id",
			"NIFCLOUD Access Key ID to fetch the metrics.",
		).Required().String()
		secretAccessKey = kingpin.Flag(
			"nifcloud.secret-access-key",
			"NIFCLOUD Secret Access Key to fetch the metrics.",
		).Required().String()
	)

	log.AddFlags(kingpin.CommandLine)
	kingpin.Version(version.Print(exporterName))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	log.Infof("Starting %s %v", exporterName, version.Info())
	log.Infoln("Build context", version.BuildContext())

	http.Handle(
		*metricsPath,
		newHandler(
			!*disableExporterMetrics, *maxRequests,
			*nasInstanceID, *accessKeyID, *secretAccessKey, *region,
		),
	)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>NIFCLOUD NAS Exporter</title></head>
			<body>
			<h1>NIFCLOUD NAS Exporter</h1>
			<p><a href="` + *metricsPath + `">Metrics</a></p>
			</body>
			</html>`))
	})

	log.Infoln("Listening on", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		log.Fatal(err)
	}
}

func newHandler(includeExporterMetrics bool, maxRequests int,
	nasInstanceIdentifier, accessKeyID, secretAccessKey, region string) http.Handler {
	exporterMetricsRegistry := prometheus.NewRegistry()

	if includeExporterMetrics {
		exporterMetricsRegistry.MustRegister(
			prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
			prometheus.NewGoCollector(),
		)
	}

	r := prometheus.NewRegistry()
	r.MustRegister(version.NewCollector(exporterName))
	r.Register(collector.NewNASCollector(nasInstanceIdentifier, accessKeyID, secretAccessKey, region))
	handler := promhttp.HandlerFor(
		prometheus.Gatherers{exporterMetricsRegistry, r},
		promhttp.HandlerOpts{
			ErrorLog:            log.NewErrorLogger(),
			ErrorHandling:       promhttp.ContinueOnError,
			MaxRequestsInFlight: maxRequests,
			Registry:            exporterMetricsRegistry,
		},
	)

	if includeExporterMetrics {
		handler = promhttp.InstrumentMetricHandler(
			exporterMetricsRegistry, handler,
		)
	}

	return handler
}
