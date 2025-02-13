package config

import (
	cconfig "github.com/descope/common/pkg/common/config"
)

const (
	ConfigKeyDirectRelationCacheSizePerProject   = "AUTHZCACHE_DIRECT_RELATION_CACHE_SIZE_PER_PROJECT"
	ConfigKeyIndirectRelationCacheSizePerProject = "AUTHZCACHE_INDIRECT_RELATION_CACHE_SIZE_PER_PROJECT"
	ConfigKeyRemotePollingIntervalInMillis       = "AUTHZCACHE_REMOTE_POLLING_INTERVAL_IN_MILLIS"
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
