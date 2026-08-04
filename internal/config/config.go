package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Agent            AgentConfig      `yaml:"agent"`
	Ecosystem        EcosystemConfig  `yaml:"ecosystem"`
	FailsafeDefaults FailsafeDefaults `yaml:"failsafe_defaults"`
	Resilience       ResilienceConfig `yaml:"resilience"`
}

type AgentConfig struct {
	Model string  `yaml:"model"`
	Temp  float32 `yaml:"temp"`
}

type EcosystemConfig struct {
	CorrectionSysPrompt string          `yaml:"correction_sys_prompt"`
	JournalSysPrompt    string          `yaml:"journal_sys_prompt"`
	BotanicalProfile    string          `yaml:"botanical_profile"`
	IdealConditions     IdealConditions `yaml:"ideal_conditions"`
}

type IdealConditions struct {
	TempDayCMin              float32 `yaml:"temp_day_c_min"`
	TempDayCMax              float32 `yaml:"temp_day_c_max"`
	TempNightCMin            float32 `yaml:"temp_night_c_min"`
	TempNightCMax            float32 `yaml:"temp_night_c_max"`
	HumidityVegPercentMin    float32 `yaml:"humidity_veg_percent_min"`
	HumidityVegPercentMax    float32 `yaml:"humidity_veg_percent_max"`
	HumidityFlowerPercentMin float32 `yaml:"humidity_flower_percent_min"`
	HumidityFlowerPercentMax float32 `yaml:"humidity_flower_percent_max"`
	LightHoursMin            uint    `yaml:"light_hours_min"`
	LightHoursMax            uint    `yaml:"light_hours_max"`
	PhMin                    float32 `yaml:"ph_min"`
	PhMax                    float32 `yaml:"ph_max"`
	MinSoilMoisturePercent   float32 `yaml:"min_soil_moisture_percent"`
	MaxSoilMoisturePercent   float32 `yaml:"max_soil_moisture_percent"`
	MaxTemperatureC          float32 `yaml:"max_temperature_c"`
	MaxHumidityPercent       float32 `yaml:"max_humidity_percent"`
}

type FailsafeDefaults struct {
	WaterPumpDurationS      uint `yaml:"water_pump_duration_s"`
	GrowLightOn             bool `yaml:"grow_light_on"`
	LightOnHourLocal        uint `yaml:"light_on_hour_local"`
	LightOffHourLocal       uint `yaml:"light_off_hour_local"`
	HeatingPadOn            bool `yaml:"relay_3_heating_pad_on"`
	IntakeFanOn             bool `yaml:"intake_fan_on"`
	ExhaustFanOn            bool `yaml:"exhaust_fan_on"`
	MaxWateringEventsPerDay uint `yaml:"max_watering_events_per_day"`
}

type ResilienceConfig struct {
	MaxCorrectionRetries       uint   `yaml:"max_correction_retries"`
	CorrectionJitterSeconds    uint   `yaml:"jitter_seconds"`
	MaxJournalRetries          uint   `yaml:"max_journal_retries"`
	JournalRetryDelayMinutes   []uint `yaml:"retry_delay_minutes"`
	JournalRetryBackoffSeconds uint   `yaml:"journal_retry_backoff_seconds"`
	DegradedAlert              bool   `yaml:"degraded_alert"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
