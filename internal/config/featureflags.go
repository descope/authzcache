package config

import "github.com/descope/common/pkg/common/featureflags"

// Feature flags
var UpgradeSchemaForFGAFeature = &featureflags.FeatureFlag{Flag: "UPGRADE_SCHEMA_FOR_FGA", DefaultVal: true}
