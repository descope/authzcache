package config

import (
	cconfig "github.com/descope/common/pkg/common/config"
)

const (
	ConfigKeySchemaCacheSize          = "AUTHZ_SCHEMA_CACHE_SIZE"
	ConfigKeyRelationCacheSize        = "AUTHZ_RELATION_CACHE_SIZE"
	ConfigKeyRelatedRelationCacheSize = "AUTHZ_RELATED_RELATION_CACHE_SIZE"
	ConfigKeyListRelationsMaxPageSize = "AUTHZ_LIST_RELATIONS_MAX_PAGE_SIZE"
	MetricsKeyResourceServiceName     = "service_name"
	MetricsKeyPartition               = "partition"
	SkipPartitionPruning              = "skip_partition_pruning"
)

func GetSchemaCacheSize() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeySchemaCacheSize, 1000)
}

func GetRelationCacheSize() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyRelationCacheSize, 1000000)
}

func GetRelatedRelationCacheSize() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyRelatedRelationCacheSize, 100000)
}

func GetListRelationsMaxPageSize() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyListRelationsMaxPageSize, 1000)
}
