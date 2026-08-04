package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"

	"sonmi/internal/api"
	"sonmi/internal/db"
)

type DummyRelayState struct {
	mu         sync.Mutex
	waterPump  bool
	growLight  bool
	intakeFan  bool
	exhaustFan bool
}

func startDummyStream(folderPath string, serverAddr string, interval time.Duration) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Println("Stream: failed to read folder:", err)
		return
	}

	var images []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") {
			images = append(images, filepath.Join(folderPath, e.Name()))
		}
	}

	if len(images) == 0 {
		fmt.Println("Stream: no JPEG images found in", folderPath)
		return
	}

	fmt.Printf("Stream: cycling through %d images at %s\n", len(images), serverAddr)

	for i := 0; ; i++ {
		path := images[i%len(images)]

		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Println("Stream: failed to read image:", err)
			time.Sleep(interval)
			continue
		}

		req, err := http.NewRequest(http.MethodPost, serverAddr+"/stream", bytes.NewReader(data))
		if err != nil {
			fmt.Println("Stream: failed to create request:", err)
			time.Sleep(interval)
			continue
		}
		req.Header.Set(api.AUTH_HEADER_FIELD, os.Getenv("MEDIA_AUTH_KEY"))
		req.Header.Set("Content-Type", "image/jpeg")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println("Stream: POST failed:", err)
			time.Sleep(interval)
			continue
		}
		resp.Body.Close()

		fmt.Printf("Stream: posted %s\n", filepath.Base(path))
		time.Sleep(interval)
	}
}

func main() {
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "local"
	}
	envFile := ".env." + appEnv
	if err := godotenv.Load(envFile); err != nil {
		log.Printf("Warning: Could not load %s: %v (relying on system env vars)", envFile, err)
	}

	interval := flag.Duration("interval", 5*time.Minute, "telemetry publish interval")
	streamFolder := flag.String("stream", "", "folder of JPEGs to cycle as dummy camera stream")
	streamAddr := flag.String("stream-addr", "http://localhost:8080", "address of the media server")
	streamInterval := flag.Duration("stream-interval", 100*time.Millisecond, "delay between frames")
	flag.Parse()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(os.Getenv("MQTT_BROKER"))
	opts.SetClientID(os.Getenv("MQTT_ESP_CLIENT_ID"))
	if username := os.Getenv("MQTT_USERNAME"); username != "" {
		opts.SetUsername(username)
		opts.SetPassword(os.Getenv("MQTT_PASSWORD"))
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	fmt.Println("Dummy ESP32 Connected to Cloud! Pumping data...")

	if *streamFolder != "" {
		go startDummyStream(*streamFolder, *streamAddr, *streamInterval)
	}

	dummyRelayState := &DummyRelayState{}

	// publishRelayState simulates the ESP32 confirming a relay pin change back to the server.
	publishRelayState := func(relay db.Relay_t, value bool, mode db.Mode_t) {
		rs := api.RelayState{
			Relay: relay,
			Value: value,
			Mode:  mode,
			Time:  time.Now().UTC(),
		}
		if payload, err := json.Marshal(rs); err == nil {
			client.Publish(os.Getenv("MQTT_TOPIC_RELAY_STATE"), 1, false, payload)
			fmt.Printf("Dummy ESP32: relay state published → %s=%v (mode=%s)\n", relay, value, mode)
		}
	}

	client.Subscribe(os.Getenv("MQTT_TOPIC_COMMANDS"), 1, func(client mqtt.Client, msg mqtt.Message) {
		var cmd api.ActuatorCommand
		json.Unmarshal(msg.Payload(), &cmd)

		dummyRelayState.mu.Lock()

		// WaterPumpDuration: duration > 0 means turn on, 0 means turn off.
		// The dummy simulates the timed behaviour: it turns the pump on, waits, then turns it off.
		if cmd.WaterPumpDuration != nil {
			duration := *cmd.WaterPumpDuration
			if duration > 0 {
				dummyRelayState.waterPump = true
				publishRelayState(db.RelayWaterPump, true, cmd.Mode)
				// Simulate the ESP32 auto-shutoff after the duration elapses
				go func() {
					time.Sleep(time.Duration(duration) * time.Second)
					dummyRelayState.mu.Lock()
					dummyRelayState.waterPump = false
					dummyRelayState.mu.Unlock()
					publishRelayState(db.RelayWaterPump, false, cmd.Mode)
				}()
			} else {
				dummyRelayState.waterPump = false
				publishRelayState(db.RelayWaterPump, false, cmd.Mode)
			}
		}
		if cmd.GrowLight != nil {
			dummyRelayState.growLight = *cmd.GrowLight
			publishRelayState(db.RelayGrowLight, *cmd.GrowLight, cmd.Mode)
		}
		if cmd.IntakeFan != nil {
			dummyRelayState.intakeFan = *cmd.IntakeFan
			publishRelayState(db.RelayIntakeFan, *cmd.IntakeFan, cmd.Mode)
		}
		if cmd.ExhaustFan != nil {
			dummyRelayState.exhaustFan = *cmd.ExhaustFan
			publishRelayState(db.RelayExhaustFan, *cmd.ExhaustFan, cmd.Mode)
		}

		dummyRelayState.mu.Unlock()
	})

	// Dummy ESP32 -> Server Loop
	for {

		// Generate fake JSON payload
		dummyRelayState.mu.Lock()
		payload := api.Telemetry{
			Sensors: api.Sensors{
				Temperature:  20.0 + float32(rand.Intn(10)),
				AirHumidity:  40.0 + float32(rand.Intn(20)),
				SoilHumidity: 30.0 + float32(rand.Intn(30)),
			},
			Relays: api.Relays{
				WaterPump:  dummyRelayState.waterPump,
				GrowLight:  dummyRelayState.growLight,
				IntakeFan:  dummyRelayState.intakeFan,
				ExhaustFan: dummyRelayState.exhaustFan,
			},
		}
		dummyRelayState.mu.Unlock()

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			fmt.Println("Failed to marshal payload:", err)
			continue
		}

		client.Publish(os.Getenv("MQTT_TOPIC_TELEMETRY"), 1, false, jsonPayload)

		fmt.Println("Sent:", payload)

		time.Sleep(*interval)
	}
}
