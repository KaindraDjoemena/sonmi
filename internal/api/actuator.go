package api

import (
	"os"

	"sonmi/internal/db"
)

// JSON payload to esp32 via MQTT
// WaterPumpDuration is in seconds; 0 means off, >0 means run for that many seconds.
// Mode is echoed back by the ESP32 in its relay_state confirmation for accurate audit logging.
type ActuatorCommand struct {
	WaterPumpDuration *uint     `json:"waterPumpDuration,omitempty"` // 0: off, >0: on duration
	GrowLight         *bool     `json:"growLight,omitempty"`
	IntakeFan         *bool     `json:"intakeFan,omitempty"`
	ExhaustFan        *bool     `json:"exhaustFan,omitempty"`
	Mode              db.Mode_t `json:"mode"`
}

// DeviceController defines the interface for sending commands to the physical hardware.
// Logging to the database is NOT performed here; it is done reactively by [StartRelayStateLogger]
// when the hardware confirms the state change via MQTT.
type DeviceController interface {
	ToggleWaterPump(duration uint, toggleMode db.Mode_t, rationale string) error
	ToggleGrowLight(state bool, toggleMode db.Mode_t, rationale string) error
	ToggleIntakeFan(state bool, toggleMode db.Mode_t, rationale string) error
	ToggleExhaustFan(state bool, toggleMode db.Mode_t, rationale string) error
}

// ActuatorController implements DeviceController and publishes commands to the MQTT broker.
type ActuatorController struct {
	Client *MQTTClient
}

func (c ActuatorController) ToggleWaterPump(duration uint, toggleMode db.Mode_t, rationale string) error {
	cmd := ActuatorCommand{WaterPumpDuration: &duration, Mode: toggleMode}
	return c.Client.SendCommand(cmd, os.Getenv("MQTT_TOPIC_COMMANDS"))
}

func (c ActuatorController) ToggleGrowLight(state bool, toggleMode db.Mode_t, rationale string) error {
	cmd := ActuatorCommand{GrowLight: &state, Mode: toggleMode}
	return c.Client.SendCommand(cmd, os.Getenv("MQTT_TOPIC_COMMANDS"))
}

func (c ActuatorController) ToggleIntakeFan(state bool, toggleMode db.Mode_t, rationale string) error {
	cmd := ActuatorCommand{IntakeFan: &state, Mode: toggleMode}
	return c.Client.SendCommand(cmd, os.Getenv("MQTT_TOPIC_COMMANDS"))
}

func (c ActuatorController) ToggleExhaustFan(state bool, toggleMode db.Mode_t, rationale string) error {
	cmd := ActuatorCommand{ExhaustFan: &state, Mode: toggleMode}
	return c.Client.SendCommand(cmd, os.Getenv("MQTT_TOPIC_COMMANDS"))
}
