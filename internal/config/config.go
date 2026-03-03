package config

import (
	cconfig "github.com/descope/common/pkg/common/config"
)

const (
	ConfigKeyDirectRelationCacheSizePerProject   = "AUTHZCACHE_DIRECT_RELATION_CACHE_SIZE_PER_PROJECT"
	ConfigKeyIndirectRelationCacheSizePerProject = "AUTHZCACHE_INDIRECT_RELATION_CACHE_SIZE_PER_PROJECT"
	ConfigKeyRemotePollingIntervalInMillis       = "AUTHZCACHE_REMOTE_POLLING_INTERVAL_IN_MILLIS"
	ConfigKeyPurgeCooldownWindowInMinutes        = "AUTHZCACHE_PURGE_COOLDOWN_WINDOW_IN_MINUTES"
	ConfigKeySDKDebugLog                         = "AUTHZCACHE_SDK_DEBUG_LOG" // TRUE/FALSE, default is FALSE
	MetricsKeyResourceServiceName                = "service_name"

	// Lookup cache configuration
	ConfigKeyLookupCacheEnabled        = "AUTHZCACHE_LOOKUP_CACHE_ENABLED"          // TRUE/FALSE, default is TRUE
	ConfigKeyLookupCacheSizePerProject = "AUTHZCACHE_LOOKUP_CACHE_SIZE_PER_PROJECT" // max entries per project
	ConfigKeyLookupCacheTTLInSeconds   = "AUTHZCACHE_LOOKUP_CACHE_TTL_IN_SECONDS"   // TTL for lookup cache entries
	ConfigKeyLookupCacheMaxResultSize  = "AUTHZCACHE_LOOKUP_CACHE_MAX_RESULT_SIZE"  // max result size to cache (skip caching large results)

	// Metrics reporting configuration
	ConfigKeyMetricsReportEnabled           = "AUTHZCACHE_METRICS_REPORT_ENABLED"             // TRUE/FALSE, default is TRUE
	ConfigKeyMetricsReportIntervalInSeconds = "AUTHZCACHE_METRICS_REPORT_INTERVAL_IN_SECONDS" // interval in seconds, default is 60, min is 10
)

func GetDirectRelationCacheSizePerProject() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyDirectRelationCacheSizePerProject, 1_000_000)
}

func GetIndirectRelationCacheSizePerProject() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyIndirectRelationCacheSizePerProject, 1_000_000)
}

func GetRemotePollingIntervalInMillis() int {
	return max(15_000, cconfig.GetIntOrProvidedLocal(ConfigKeyRemotePollingIntervalInMillis, 15_000))
}

func GetSDKDebugLog() bool {
	return cconfig.GetBoolOrProvidedLocal(ConfigKeySDKDebugLog, false)
}

func GetPurgeCooldownWindowInMinutes() int {
	return max(0, cconfig.GetIntOrProvidedLocal(ConfigKeyPurgeCooldownWindowInMinutes, 0))
}

func GetLookupCacheEnabled() bool {
	return cconfig.GetBoolOrProvidedLocal(ConfigKeyLookupCacheEnabled, true)
}

func GetLookupCacheSizePerProject() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyLookupCacheSizePerProject, 10_000)
}

func GetLookupCacheTTLInSeconds() int {
	return max(1, cconfig.GetIntOrProvidedLocal(ConfigKeyLookupCacheTTLInSeconds, 60))
}

func GetLookupCacheMaxResultSize() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyLookupCacheMaxResultSize, 1000)
}

func GetMetricsReportEnabled() bool {
	return cconfig.GetBoolOrProvidedLocal(ConfigKeyMetricsReportEnabled, true)
}

func GetMetricsReportIntervalInSeconds() int {
	return max(10, cconfig.GetIntOrProvidedLocal(ConfigKeyMetricsReportIntervalInSeconds, 60))
}
