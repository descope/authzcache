package config

import (
	cconfig "github.com/descope/common/pkg/common/config"
)

const (
	ConfigKeySchemaCacheSize                      = "AUTHZ_SCHEMA_CACHE_SIZE"
	ConfigKeyDirectRelationCacheSize              = "AUTHZ_DIRECT_RELATION_CACHE_SIZE"
	ConfigKeyIndirectAndNegativeRelationCacheSize = "AUTHZ_INDIRECT_AND_NEGATIVE_RELATION_CACHE_SIZE"
	MetricsKeyResourceServiceName                 = "service_name"
)

func GetSchemaCacheSize() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeySchemaCacheSize, 1000)
}

func GetDirectRelationCacheSize() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyDirectRelationCacheSize, 1_000_000)
}

func GetInderectAndNegativeRelationCacheSize() int {
	return cconfig.GetIntOrProvidedLocal(ConfigKeyIndirectAndNegativeRelationCacheSize, 1_000_000)
}
