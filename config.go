package ghalint

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Config configures rules and cost estimation.
type Config struct {
	Disabled       []string           `json:"disabled"`
	AllowTagOwners []string           `json:"allowTagOwners"`
	DefaultTimeout int                `json:"defaultTimeout"`
	Multipliers    map[string]float64 `json:"multipliers"`
}

type rawConfig struct {
	Disabled       []string           `json:"disabled"`
	AllowTagOwners []string           `json:"allowTagOwners"`
	DefaultTimeout *int               `json:"defaultTimeout"`
	Multipliers    map[string]float64 `json:"multipliers"`
}

// DefaultConfig returns the defaults: nothing disabled, no exempt owners,
// a 360 minute timeout and multipliers linux 1, windows 2, macos 10.
func DefaultConfig() Config {
	return Config{Disabled: []string{}, AllowTagOwners: []string{}, DefaultTimeout: 360, Multipliers: map[string]float64{"linux": 1, "windows": 2, "macos": 10}}
}

func (c Config) normalized() Config {
	d := DefaultConfig()
	if c.DefaultTimeout > 0 {
		d.DefaultTimeout = c.DefaultTimeout
	}
	for k, v := range c.Multipliers {
		if v > 0 {
			d.Multipliers[k] = v
		}
	}
	d.Disabled = c.Disabled
	d.AllowTagOwners = c.AllowTagOwners
	return d
}

func cfgErr(msg string) error {
	return &LintError{Code: "config", Msg: short(msg)}
}

// LoadConfig parses a JSON configuration on top of the defaults. Invalid
// input fails with ErrConfig; input over MaxFileBytes with ErrTooLarge.
func LoadConfig(data []byte) (Config, error) {
	if len(data) > MaxFileBytes {
		return Config{}, &LintError{Code: "too_large", Msg: "config exceeds 1 MiB"}
	}
	var raw rawConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return Config{}, cfgErr(err.Error())
	}
	if dec.More() {
		return Config{}, cfgErr("trailing data after configuration")
	}
	cfg := DefaultConfig()
	if raw.DefaultTimeout != nil {
		if *raw.DefaultTimeout <= 0 {
			return Config{}, cfgErr("defaultTimeout must be positive")
		}
		cfg.DefaultTimeout = *raw.DefaultTimeout
	}
	for k, v := range raw.Multipliers {
		if k != "linux" && k != "windows" && k != "macos" {
			return Config{}, cfgErr("unknown multiplier " + k)
		}
		if !(v > 0) {
			return Config{}, cfgErr("multiplier " + k + " must be greater than 0")
		}
		cfg.Multipliers[k] = v
	}
	for _, d := range raw.Disabled {
		cfg.Disabled = append(cfg.Disabled, strings.ToUpper(d))
	}
	cfg.AllowTagOwners = append(cfg.AllowTagOwners, raw.AllowTagOwners...)
	return cfg, nil
}
