package api

type Telemetry struct {
	Sensors Sensors `json:"sensors"`
	Relays  Relays  `json:"relays"`
}

type Sensors struct {
	Temperature  float32 `json:"temperature"`
	AirHumidity  float32 `json:"airHumidity"`
	SoilHumidity float32 `json:"soilHumidity"`
}

type Relays struct {
	WaterPump  bool `json:"waterPump"`
	GrowLight  bool `json:"growLight"`
	IntakeFan  bool `json:"intakeFan"`
	ExhaustFan bool `json:"exhaustFan"`
}
