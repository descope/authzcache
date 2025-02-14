package config

import (
	cconfig "github.com/descope/common/pkg/common/config"
)

const (
	ConfigKeyDirectRelationCacheSizePerProject   = "AUTHZCACHE_DIRECT_RELATION_CACHE_SIZE_PER_PROJECT"
	ConfigKeyIndirectRelationCacheSizePerProject = "AUTHZCACHE_INDIRECT_RELATION_CACHE_SIZE_PER_PROJECT"
	ConfigKeyRemotePollingIntervalInMillis       = "AUTHZCACHE_REMOTE_POLLING_INTERVAL_IN_MILLIS"
	ConfigKeySDKDebugLog                         = "AUTHZCACHE_SDK_DEBUG_LOG" // TRUE/FALSE, default is FALSE
	MetricsKeyResourceServiceName                = "service_name"
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
