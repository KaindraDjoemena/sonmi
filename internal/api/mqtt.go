package api

import (
	"encoding/json"
	"log"
	"os"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MQTTClient struct {
	mqttClient mqtt.Client
}

// NewClient connects to the MQTT broker, subscribes to the telemetry topic, the
// relay state topic, and the gateway health topic, and returns a ready-to-use MQTTClient.
func NewClient(telemetryPipe chan Telemetry, relayStatePipe chan RelayState, gatewayHealthPipe chan GatewayHealth, broker string, clientId string, telemetryTopic string) (*MQTTClient, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientId)
	if username := os.Getenv("MQTT_USERNAME"); username != "" {
		opts.SetUsername(username)
		opts.SetPassword(os.Getenv("MQTT_PASSWORD"))
	}

	// Initialize Client
	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	// Subscribe to sensor telemetry
	mqttClient.Subscribe(telemetryTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		var data Telemetry

		if err := json.Unmarshal(msg.Payload(), &data); err != nil {
			log.Printf("Failed to Unmarshal Telemetry JSON: %v\n", err)
			return
		}

		telemetryPipe <- data // [StartTelemetryLogger] consumes the channel
	})

	// Subscribe to hardware relay state confirmations
	mqttClient.Subscribe(os.Getenv("MQTT_TOPIC_RELAY_STATE"), 1, func(client mqtt.Client, msg mqtt.Message) {
		var data RelayState

		if err := json.Unmarshal(msg.Payload(), &data); err != nil {
			log.Printf("Failed to Unmarshal Relay State JSON: %v\n", err)
			return
		}

		relayStatePipe <- data // [StartRelayStateLogger] consumes the channel
	})

	// Subscribe to the Pi edge gateway's health reports (roadmap.md Phase 4)
	if topic := os.Getenv("MQTT_TOPIC_GATEWAY_HEALTH"); topic != "" {
		mqttClient.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			var data GatewayHealth

			if err := json.Unmarshal(msg.Payload(), &data); err != nil {
				log.Printf("Failed to Unmarshal Gateway Health JSON: %v\n", err)
				return
			}

			select {
			case gatewayHealthPipe <- data:
			default: // non-blocking: drop if the TUI isn't currently reading
			}
		})
	}

	return &MQTTClient{
		mqttClient: mqttClient,
	}, nil
}

func (c *MQTTClient) SendCommand(cmd ActuatorCommand, pubTopic string) error {
	jsonPayload, err := json.Marshal(cmd)
	if err != nil {
		log.Printf("Failed to Marshal Actuator Command: %v\n", err)
		return err
	}

	if token := c.mqttClient.Publish(pubTopic, 1, false, jsonPayload); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}
