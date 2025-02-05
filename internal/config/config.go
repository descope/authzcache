package config

import (
	cconfig "github.com/descope/common/pkg/common/config"
)

const (
	ConfigKeyDirectRelationCacheSizePerProject              = "AUTHZCACHE_DIRECT_RELATION_CACHE_SIZE_PER_PROJECT"
	ConfigKeyIndirectAndNegativeRelationCacheSizePerProject = "AUTHZCACHE_INDIRECT_AND_NEGATIVE_RELATION_CACHE_SIZE_PER_PROJECT"
	ConfigKeyRemotePollingIntervalInSeconds                 = "AUTHZCACHE_REMOTE_POLLING_INTERVAL_IN_SECONDS"
	MetricsKeyResourceServiceName                           = "service_name"
)

func GetDirectRelationCacheSizePerProject() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyDirectRelationCacheSizePerProject, 1_000_000)
}

func GetInderectAndNegativeRelationCacheSizePerProject() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyIndirectAndNegativeRelationCacheSizePerProject, 1_000_000)
}

func GetRemotePollingIntervalInSeconds() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyRemotePollingIntervalInSeconds, 15)
}
