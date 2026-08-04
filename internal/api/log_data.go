package api

import (
	"log"
	"time"

	"sonmi/internal/db"
)

// RelayState is the payload published by the ESP32 to MQTT_TOPIC_RELAY_STATE
// whenever a physical relay pin changes state. This is the hardware's ground truth.
// Mode is echoed from the original ActuatorCommand for accurate audit trail logging.
type RelayState struct {
	Relay db.Relay_t `json:"relay"`
	Value bool       `json:"value"`
	Mode  db.Mode_t  `json:"mode"`
	Time  time.Time  `json:"time"`
}

// Consume the Telemetry -> write to DB -> pass the Telemetry to the UI
func StartTelemetryLogger(internalPipe <-chan Telemetry, tuiPipe chan<- Telemetry, database db.Database) {

	// Consume from [NewClient]
	for t := range internalPipe {
		newTelemetryRow := db.SensorTelemetryRow{
			Temp:         t.Sensors.Temperature,
			AirHumidity:  t.Sensors.AirHumidity,
			SoilHumidity: t.Sensors.SoilHumidity,
			Time:         time.Now(),
		}

		if err := newTelemetryRow.Insert(database); err != nil {
			log.Printf("Telemetry Logging Error: %v\n", err)
			continue
		}

		select {
		case tuiPipe <- t: // Pass to [tui.waitForTelemetry]
		default:
		}
	}
}

// Consume the RelayState -> write to DB -> pass the RelayState to the UI
func StartRelayStateLogger(internalPipe <-chan RelayState, tuiPipe chan<- RelayState, database db.Database) {

	// Consume from [NewClient]
	for rs := range internalPipe {
		row := db.RelayEventRow{
			Relay:     rs.Relay,
			Mode:      rs.Mode,
			Value:     rs.Value,
			Rationale: "",
			Time:      rs.Time,
		}

		if err := row.Insert(database); err != nil {
			log.Printf("Relay State Logging Error: %v\n", err)
			continue
		}

		select {
		case tuiPipe <- rs: // Pass to [tui.waitForRelayState]
		default:
		}
	}
}
