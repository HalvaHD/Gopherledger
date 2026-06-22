// Package config Пакет config отвечает за загрузку конфигурации приложения из YAML.
package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

// Config тут описаны все поля из config.yml
type Config struct {
	ServerHost      string `yaml:"server_host"`
	ServerPort      int    `yaml:"server_port"`
	LogLevel        string `yaml:"log_level"`
	AccrualInterval int    `yaml:"accrual_interval_seconds"`
	WorkerCount     int    `yaml:"worker_concurrency"`
}

var MyConfiguration *Config

// defaultConfig фолбек, на случай отсутвия файла
func defaultConfig() *Config {
	return &Config{
		ServerHost:      "localhost",
		ServerPort:      8080,
		LogLevel:        "info",
		AccrualInterval: 3,
		WorkerCount:     5,
	}
}

// Load читает config.yaml
func Load() (*Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile("config.yaml")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}
	// Здесь ямлик файл важнее всего
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	applyFallbacks(cfg)
	return cfg, nil
}

// applyFallbacks фолбек на случай отсутвия файла (на всякий случай)
func applyFallbacks(c *Config) {
	if c.ServerHost == "" {
		c.ServerHost = "0.0.0.0"
	}
	if c.ServerPort == 0 {
		c.ServerPort = 8080
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.AccrualInterval <= 0 {
		c.AccrualInterval = 3
	}
	if c.WorkerCount <= 0 {
		c.WorkerCount = 5
	}
}
