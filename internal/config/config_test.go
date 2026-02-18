package config

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRemotePollingIntervalInMillis(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		expected int
	}{
		{
			name:     "Config value greater than minimum",
			interval: 20_000,
			expected: 20_000,
		},
		{
			name:     "Config value less than minimum",
			interval: 10_000,
			expected: 15_000,
		},
		{
			name:     "Config value not set",
			expected: 15_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.interval > 0 {
				t.Setenv(ConfigKeyRemotePollingIntervalInMillis, strconv.Itoa(tt.interval))
			}
			actual := GetRemotePollingIntervalInMillis()
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetDirectRelationCacheSizePerProject(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		expected int
	}{
		{
			name:     "Config value set",
			size:     2_000_000,
			expected: 2_000_000,
		},
		{
			name:     "Config value not set",
			expected: 1_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.size > 0 {
				t.Setenv(ConfigKeyDirectRelationCacheSizePerProject, strconv.Itoa(tt.size))
			}
			actual := GetDirectRelationCacheSizePerProject()
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetIndirectRelationCacheSizePerProject(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		expected int
	}{
		{
			name:     "Config value set",
			size:     2_000_000,
			expected: 2_000_000,
		},
		{
			name:     "Config value not set",
			expected: 1_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.size > 0 {
				t.Setenv(ConfigKeyIndirectRelationCacheSizePerProject, strconv.Itoa(tt.size))
			}
			actual := GetIndirectRelationCacheSizePerProject()
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetSDKDebugLog(t *testing.T) {
	tests := []struct {
		name     string
		debug    string
		expected bool
	}{
		{
			name:     "Config value set to true",
			debug:    "true",
			expected: true,
		},
		{
			name:     "Config value set to TRUE",
			debug:    "TRUE",
			expected: true,
		},
		{
			name:     "Config value set to false",
			debug:    "false",
			expected: false,
		},
		{
			name:     "Config value set to FALSE",
			debug:    "FALSE",
			expected: false,
		},
		{
			name:     "Config value not set",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ConfigKeySDKDebugLog, tt.debug)
			actual := GetSDKDebugLog()
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetPurgeCooldownWindowInMinutes(t *testing.T) {
	tests := []struct {
		name     string
		cooldown int
		expected int
	}{
		{
			name:     "Config value set to positive",
			cooldown: 30,
			expected: 30,
		},
		{
			name:     "Config value set to zero",
			cooldown: 0,
			expected: 0,
		},
		{
			name:     "Config value set to negative (should return 0)",
			cooldown: -5,
			expected: 0,
		},
		{
			name:     "Config value not set (default 0)",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name != "Config value not set (default 0)" {
				t.Setenv(ConfigKeyPurgeCooldownWindowInMinutes, strconv.Itoa(tt.cooldown))
			}
			actual := GetPurgeCooldownWindowInMinutes()
			require.Equal(t, tt.expected, actual)
		})
	}
}
