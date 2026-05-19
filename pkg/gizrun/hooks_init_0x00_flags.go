package gizrun

import "flag"

func init() {
	InitAt(0x00, func() error {
		flag.StringVar(&runCtx.configPath, "config", runCtx.configPath, "config file path")

		flag.IntVar(&runCtx.debugPort, "debug-port", runCtx.debugPort, "debug Fiber app listen port")
		flag.StringVar(&runCtx.addr, "listen", runCtx.addr, "public Fiber app listen address")
		flag.BoolVar(&runCtx.enablePprof, "pprof", false, "enable pprof handlers")
		flag.Var(&runCtx.verbose, "verbose", "set log verbosity: debug, info, warn, or error")

		flag.BoolVar(&runCtx.metricsPush.Enabled, "metrics-push-enabled", runCtx.metricsPush.Enabled, "enable metrics push")
		flag.StringVar(&runCtx.metricsPush.URL, "metrics-push-url", runCtx.metricsPush.URL, "metrics push endpoint")
		flag.StringVar(&runCtx.metricsPush.Job, "metrics-push-job", runCtx.metricsPush.Job, "metrics push job")
		flag.StringVar(&runCtx.metricsPush.Username, "metrics-push-username", runCtx.metricsPush.Username, "metrics push basic auth username")
		flag.StringVar(&runCtx.metricsPush.Password, "metrics-push-password", runCtx.metricsPush.Password, "metrics push basic auth password")
		flag.StringVar(&runCtx.metricsPush.BearerToken, "metrics-push-bearer-token", runCtx.metricsPush.BearerToken, "metrics push bearer token")
		flag.StringVar(&runCtx.metricsPush.Interval, "metrics-push-interval", runCtx.metricsPush.Interval, "metrics push interval")
		flag.Var(&runCtx.metricsPush.Grouping, "metrics-push-grouping", "metrics push grouping label as key=value; repeatable")

		flag.BoolVar(&runCtx.volcLog.Enabled, "volc-log-enabled", runCtx.volcLog.Enabled, "enable Volcengine TLS log sink")
		flag.StringVar(&runCtx.volcLog.Endpoint, "volc-log-endpoint", runCtx.volcLog.Endpoint, "Volcengine TLS endpoint")
		flag.StringVar(&runCtx.volcLog.Region, "volc-log-region", runCtx.volcLog.Region, "Volcengine TLS region")
		flag.StringVar(&runCtx.volcLog.AccessKeyID, "volc-log-access-key-id", runCtx.volcLog.AccessKeyID, "Volcengine access key id")
		flag.StringVar(&runCtx.volcLog.AccessKeySecret, "volc-log-access-key-secret", runCtx.volcLog.AccessKeySecret, "Volcengine access key secret")
		flag.StringVar(&runCtx.volcLog.SecurityToken, "volc-log-security-token", runCtx.volcLog.SecurityToken, "Volcengine security token")
		flag.StringVar(&runCtx.volcLog.APIKey, "volc-log-api-key", runCtx.volcLog.APIKey, "Volcengine API key")
		flag.StringVar(&runCtx.volcLog.TopicID, "volc-log-topic-id", runCtx.volcLog.TopicID, "Volcengine TLS topic id")
		flag.StringVar(&runCtx.volcLog.Source, "volc-log-source", runCtx.volcLog.Source, "Volcengine TLS log source")
		flag.StringVar(&runCtx.volcLog.FileName, "volc-log-file-name", runCtx.volcLog.FileName, "Volcengine TLS log file name")
		flag.StringVar(&runCtx.volcLog.ShardHash, "volc-log-shard-hash", runCtx.volcLog.ShardHash, "Volcengine TLS shard hash")
		flag.BoolVar(&runCtx.volcLog.AddSource, "volc-log-add-source", runCtx.volcLog.AddSource, "enable Volcengine TLS add source")
		flag.BoolVar(&runCtx.volcLog.EnableNanosecond, "volc-log-enable-nanosecond", runCtx.volcLog.EnableNanosecond, "enable Volcengine TLS nanosecond timestamp")
		return nil
	})
}
