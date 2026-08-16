package main

import (
	"image"
	"log"
	"os"

	"sonmi/internal/agent"
	"sonmi/internal/api"
	"sonmi/internal/config"
	"sonmi/internal/db"
	"sonmi/internal/tui"

	"github.com/joho/godotenv"
)

const (
	FRAME_BUFFER_SIZE = 5

	DEFAULT_SSH_ADDR          = "0.0.0.0:2222"
	DEFAULT_SSH_HOST_KEY_PATH = ".ssh/sonmi_host_key"
)

func main() {
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "local"
	}
	envFile := ".env." + appEnv
	if err := godotenv.Load(envFile); err != nil {
		log.Printf("Warning: Could not load %s: %v (relying on system env vars)", envFile, err)
	}
	log.Printf("Loaded env: %s", envFile)

	// Image Stream
	framePipe := make(chan image.Image, FRAME_BUFFER_SIZE)

	// Telemetry Data
	internalTelemetryPipe := make(chan api.Telemetry)
	tuiTelemetryPipe := make(chan api.Telemetry)

	// Relay State (hardware-confirmed state changes from ESP32)
	internalRelayStatePipe := make(chan api.RelayState)
	tuiRelayStatePipe := make(chan api.RelayState)

	// Pi edge gateway health reports — consumed directly by the TUI, no DB persistence
	tuiGatewayHealthPipe := make(chan api.GatewayHealth)

	// Tracks correction/journal agent loop attempt/success times for the TUI monitor panel
	loopStatus := &api.LoopStatus{}

	// MQTT Client
	mqttClient, err := api.NewClient(
		internalTelemetryPipe,
		internalRelayStatePipe,
		tuiGatewayHealthPipe,
		os.Getenv("MQTT_BROKER"),
		os.Getenv("MQTT_SERVER_CLIENT_ID"),
		os.Getenv("MQTT_TOPIC_TELEMETRY"),
	)

	if err != nil {
		log.Fatal("Failed to connect to MQTT: ", err.Error())
		os.Exit(1)
	}

	// DB
	dbConn, err := db.NewDatabase(os.Getenv("DB_PATH"))
	if err != nil {
		log.Fatal("Failed to connect to DB: ", err.Error())
		os.Exit(1)
	}

	defer dbConn.CloseConn()

	go api.StartHTTPServer(":8080", framePipe, dbConn)

	// S3  must be initialised before the snapshot ticker starts
	if err := api.InitS3Client(); err != nil {
		log.Printf("Warning: Failed to initialise S3 client, daily snapshot ticker will not start: %v", err)
		os.Exit(1)
	} else {
		go api.StartDailySnapshotTicker(framePipe, dbConn)
		go api.StartDBBackupTicker(dbConn)
	}

	controller := api.ActuatorController{
		Client: mqttClient,
	}

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatal("Failed to load config: ", err.Error())
		os.Exit(1)
	}

	go agent.StartCorrectionLoop(dbConn, controller, cfg, loopStatus)
	go agent.StartJournalLoop(dbConn, cfg, loopStatus)
	go agent.StartRetryWorker(dbConn, cfg, loopStatus)

	go api.StartTelemetryLogger(internalTelemetryPipe, tuiTelemetryPipe, dbConn)
	go api.StartRelayStateLogger(internalRelayStatePipe, tuiRelayStatePipe, dbConn)

	sshAddr := os.Getenv("SSH_ADDR")
	if sshAddr == "" {
		sshAddr = DEFAULT_SSH_ADDR
	}

	hostKeyPath := os.Getenv("SSH_HOST_KEY_PATH")
	if hostKeyPath == "" {
		hostKeyPath = DEFAULT_SSH_HOST_KEY_PATH
	}

	// StartSSHServer blocks until SIGINT/SIGTERM — this is the app's main blocking call.
	tui.StartSSHServer(sshAddr, hostKeyPath, framePipe, tuiTelemetryPipe, tuiRelayStatePipe, tuiGatewayHealthPipe, controller, dbConn, loopStatus)
}
