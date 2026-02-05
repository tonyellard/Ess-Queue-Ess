// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the Ess-Queue-Ess configuration
type Config struct {
	Server ServerConfig  `yaml:"server"`
	Queues []QueueConfig `yaml:"queues"`
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// QueueConfig represents a queue to be created at startup
type QueueConfig struct {
	Name                   string            `yaml:"name"`
	VisibilityTimeout      int               `yaml:"visibility_timeout"`        // seconds, default 30
	MessageRetentionPeriod int               `yaml:"message_retention_period"`  // seconds, default 345600 (4 days)
	MaximumMessageSize     int               `yaml:"maximum_message_size"`      // bytes, default 262144 (256KB)
	MaxReceiveCount        int               `yaml:"max_receive_count"`         // default 3
	DelaySeconds           int               `yaml:"delay_seconds"`             // default 0
	ReceiveMessageWaitTime int               `yaml:"receive_message_wait_time"` // seconds, default 0
	Attributes             map[string]string `yaml:"attributes"`                // additional custom attributes
}

// LoadConfig reads and parses the YAML configuration file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	if config.Server.Port == 0 {
		config.Server.Port = 9324
	}
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}

	// Apply queue defaults
	for i := range config.Queues {
		q := &config.Queues[i]
		if q.VisibilityTimeout == 0 {
			q.VisibilityTimeout = 30
		}
		if q.MessageRetentionPeriod == 0 {
			q.MessageRetentionPeriod = 345600 // 4 days
		}
		if q.MaximumMessageSize == 0 {
			q.MaximumMessageSize = 262144 // 256KB
		}
		if q.MaxReceiveCount == 0 {
			q.MaxReceiveCount = 3
		}
		if q.Attributes == nil {
			q.Attributes = make(map[string]string)
		}
	}

	return &config, nil
}

// BootstrapQueues creates queues defined in the configuration
func BootstrapQueues(config *Config) error {
	for _, queueCfg := range config.Queues {
		queue, err := queueManager.CreateQueue(queueCfg.Name, queueCfg.Attributes)
		if err != nil {
			return fmt.Errorf("failed to create queue %s: %w", queueCfg.Name, err)
		}

		// Apply queue configuration
		queue.VisibilityTimeout = queueCfg.VisibilityTimeout
		queue.MessageRetentionPeriod = queueCfg.MessageRetentionPeriod
		queue.MaximumMessageSize = queueCfg.MaximumMessageSize
		queue.MaxReceiveCount = queueCfg.MaxReceiveCount
		queue.DelaySeconds = queueCfg.DelaySeconds
		queue.ReceiveMessageWaitTime = queueCfg.ReceiveMessageWaitTime
	}
	return nil
}
