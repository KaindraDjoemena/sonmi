#include <Arduino.h>
#include <DHT.h>
#include <WiFi.h>
#include <PubSubClient.h>
#include <ArduinoJson.h>
#include <array>
#include <string>

using u8  = uint8_t;
using u16 = uint16_t;
using u32 = uint32_t;

// --- CONFIGURATION ---
#include "secrets.h"

// --- PINS ---
enum class RelayPins : u8
{
    GROW_LIGHT  = 32,
    WATER_PUMP  = 26,
    INTAKE_FAN  = 27,
    EXHAUST_FAN = 14
};

enum class SensorPins : u8
{
    TEMP_AIR_HUMIDITY = 4,
    SOIL_HUMIDITY     = 34
};

constexpr int BLINKING_PIN  = 16;
constexpr int TELEMETRY_PIN = 17;

namespace Time
{
    constexpr u32 SECOND = 1000;
    constexpr u32 MINUTE = 60 * SECOND;
    constexpr u32 HOUR   = 60 * MINUTE;
    constexpr u32 DAY    = 24 * HOUR;
}

constexpr u8 numRelays = 4;
constexpr std::array<RelayPins, numRelays> relays = { RelayPins::GROW_LIGHT, RelayPins::WATER_PUMP, RelayPins::INTAKE_FAN, RelayPins::EXHAUST_FAN };

constexpr u8 numSensors = 1;
constexpr std::array<SensorPins, numSensors> sensors = { SensorPins::SOIL_HUMIDITY };


// --- SENSORS ---
class SonDHT22
{
public:
    SonDHT22(SensorPins pin)
    : _dht(static_cast<u8>(pin), DHT22)
    {
    }
    
    void Init()
    {
        _dht.begin();
    }

    bool Read()
    {
        u32 currMillis = millis();
        if (currMillis - _lastReadTimeMS >= _readInterval)
        {
            float t = _dht.readTemperature();
            float h = _dht.readHumidity();
            if (!isnan(t) && !isnan(h))
            {
                _reading.tempC = t;
                _reading.humidity = h;
            }
            _lastReadTimeMS = currMillis;

            return true;
        }

        return false;
    }

    float getTempC() const 
    {
        return _reading.tempC;
    }
    
    float getHumidity() const
    {
        return _reading.humidity;
    }

private:
    DHT _dht;
    u32 _lastReadTimeMS{0};
    u32 _readInterval = 2 * Time::SECOND;
    struct Reading
    {
        float tempC{0.0f};
        float humidity{0.0f};
    } _reading;
};

class SoilMoisture
{
public:
    SoilMoisture(SensorPins pin)
    : _pin(static_cast<u8>(pin))
    {
    }

    void Init()
    {
        analogSetPinAttenuation(_pin, ADC_11db);
    }

    bool Read()
    {
        u32 currMillis = millis();
        if (currMillis - _lastReadTimeMS >= _readInterval)
        {
            _reading = analogRead(_pin);
            _lastReadTimeMS = currMillis;
        
            return true;
        }

        return false;
    }

    float getPercent() const
    {
        // Calibrated map: ~3350 (Dry, 0%) to ~1050 (Wet, 100%)
        float p = map(_reading, 3350, 1050, 0, 100);
    
        if (p < 0)
            p = 0;
        
        if (p > 100)
            p = 100;
        
        return p;
    }

    u16 getRaw() const
    {
        return _reading;
    }

private:
    u8  _pin;
    u32 _lastReadTimeMS{0};
    u32 _readInterval = 2 * Time::SECOND;
    u16 _reading{0};
};

SonDHT22 dht(SensorPins::TEMP_AIR_HUMIDITY);
SoilMoisture soil(SensorPins::SOIL_HUMIDITY);


// --- NETWORK ---
WiFiClient espClient;
PubSubClient mqtt(espClient);

void setupWiFi()
{
    delay(10);
    Serial.printf("\nConnecting to %s\n", WIFI_SSID);
    
    WiFi.disconnect(true);
    delay(1000);
    
    WiFi.mode(WIFI_STA);
    
    IPAddress local_IP(192, 168, 4, 10);
    IPAddress gateway(192, 168, 4, 1);
    IPAddress subnet(255, 255, 255, 0);
    
    if (!WiFi.config(local_IP, gateway, subnet))
    {
        Serial.println("STA Failed to configure");
    }

    WiFi.begin(WIFI_SSID, WIFI_PASS);
    
    int retries = 0;
    while (WiFi.status() != WL_CONNECTED)
    {
        delay(500);
        Serial.print(".");
        retries++;
        if (retries > 30)
        {
            Serial.println("\nFailed to connect. Restarting...");
            ESP.restart();
        }
    }
    Serial.printf("\nWiFi connected. IP: %s\n", WiFi.localIP().toString().c_str());
}

// --- STATE & COMMAND HANDLING ---
String currentMode  = "NOMINAL";
u32 pumpStartTimeMS = 0;
u32 pumpDurationMS  = 0;
bool isPumpRunning  = false;

void publishRelayState(const char* relay, bool state, const char* mode)
{
    JsonDocument doc;
    doc["relay"] = relay;
    doc["value"] = state;
    doc["mode"]  = mode;

    char buffer[128];
    serializeJson(doc, buffer);
    if (mqtt.publish(TOPIC_RELAY_STATE, buffer))
    {
        Serial.printf("Published relay state: %s\n", buffer);
    }
}

void mqttCallback(char* topic, byte* payload, unsigned int length)
{
    Serial.printf("Message arrived on topic: %s\n", topic);

    if (strcmp(topic, TOPIC_COMMANDS) != 0)
    {
        return;
    }

    JsonDocument doc;
    DeserializationError error = deserializeJson(doc, payload, length);
    if (error)
    {
        Serial.print("deserializeJson() failed: ");
        Serial.println(error.c_str());
        return;
    }

    if (doc["mode"].is<String>())
    {
        currentMode = doc["mode"].as<String>();
    }

    // Active-LOW logic
    if (doc["growLight"].is<bool>())
    {
        bool state = doc["growLight"];
        digitalWrite(static_cast<u8>(RelayPins::GROW_LIGHT), state ? LOW : HIGH);
        publishRelayState("GROW_LIGHT", state, currentMode.c_str());
    }

    if (doc["intakeFan"].is<bool>())
    {
        bool state = doc["intakeFan"];
        digitalWrite(static_cast<u8>(RelayPins::INTAKE_FAN), state ? LOW : HIGH);
        publishRelayState("INTAKE_FAN", state, currentMode.c_str());
    }

    if (doc["exhaustFan"].is<bool>())
    {
        bool state = doc["exhaustFan"];
        digitalWrite(static_cast<u8>(RelayPins::EXHAUST_FAN), state ? LOW : HIGH);
        publishRelayState("EXHAUST_FAN", state, currentMode.c_str());
    }

    if (doc["waterPumpDuration"].is<u32>())
    {
        u32 durationSec = doc["waterPumpDuration"];
        if (durationSec > 0) // on
        {
            digitalWrite(static_cast<u8>(RelayPins::WATER_PUMP), LOW);
            isPumpRunning   = true;
            pumpStartTimeMS = millis();
            pumpDurationMS  = durationSec * Time::SECOND;
            publishRelayState("WATER_PUMP", true, currentMode.c_str());
            Serial.printf("Water pump started for %u seconds\n", durationSec);
        }
        else                // off
        {
            digitalWrite(static_cast<u8>(RelayPins::WATER_PUMP), HIGH);
            isPumpRunning = false;
            publishRelayState("WATER_PUMP", false, currentMode.c_str());
            Serial.println("Water pump stopped manually.");
        }
    }
}

void reconnectMQTT()
{
    while (!mqtt.connected())
    {
        Serial.print("Attempting MQTT connection...");
        if (mqtt.connect(ESP_CLIENT_ID))
        {
            Serial.println("connected");
            mqtt.subscribe(TOPIC_COMMANDS, 1);
        }
        else
        {
            Serial.print("failed, rc=");
            Serial.print(mqtt.state());
            Serial.println(" try again in 5 seconds");
            delay(5000);
        }
    }
}


// --- MAIN LOOP ---
void setup()
{
    Serial.begin(9600);
    
    dht.Init();
    soil.Init();

    Serial.println("\n--- Sonmi Relay Controller ---");
    
    pinMode(BLINKING_PIN, OUTPUT);
    pinMode(TELEMETRY_PIN, OUTPUT);

    for (u8 i = 0; i < numRelays; i++)
    {
        digitalWrite(static_cast<u8>(relays[i]), HIGH);
        pinMode(static_cast<u8>(relays[i]), OUTPUT);
    }

    for (u8 i = 0; i < numSensors; i++)
    {
        pinMode(static_cast<u8>(sensors[i]), INPUT);
    }

    setupWiFi();
    mqtt.setServer(MQTT_BROKER, MQTT_PORT);
    mqtt.setCallback(mqttCallback);
}

void publishTelemetry()
{
    JsonDocument doc;
    JsonObject sensors = doc["sensors"].to<JsonObject>();
    sensors["temperature"]  = dht.getTempC();
    sensors["airHumidity"]  = dht.getHumidity();
    sensors["soilHumidity"] = soil.getPercent();
    
    JsonObject relays = doc["relays"].to<JsonObject>();
    relays["waterPump"]  = digitalRead(static_cast<u8>(RelayPins::WATER_PUMP)) == LOW;
    relays["growLight"]  = digitalRead(static_cast<u8>(RelayPins::GROW_LIGHT)) == LOW;
    relays["intakeFan"]  = digitalRead(static_cast<u8>(RelayPins::INTAKE_FAN)) == LOW;
    relays["exhaustFan"] = digitalRead(static_cast<u8>(RelayPins::EXHAUST_FAN)) == LOW;
    
    char buffer[384];
    serializeJson(doc, buffer);
    
    if (mqtt.publish(TOPIC_TELEMETRY, buffer))
    {
        Serial.printf("Published telemetry: %s\n", buffer);
    } 
    else
    {
        Serial.println("Failed to publish telemetry");
    }
}

void loop()
{
    if (!mqtt.connected())
    {
        reconnectMQTT();
    }
    mqtt.loop();

    dht.Read();
    soil.Read();

    // pump automatic turn off check
    if (isPumpRunning)
    {
        if (millis() - pumpStartTimeMS >= pumpDurationMS)
        {
            digitalWrite(static_cast<u8>(RelayPins::WATER_PUMP), HIGH);
            isPumpRunning = false;
            publishRelayState("WATER_PUMP", false, currentMode.c_str());
            Serial.println("Water pump auto-shutoff completed.");
        }
    }

    static u32 lastTelemetryMS = 0;
    static bool telemetryState = false;
    if (millis() - lastTelemetryMS >= 5 * Time::SECOND)
    {
        publishTelemetry();
        telemetryState = !telemetryState;
        digitalWrite(TELEMETRY_PIN, telemetryState ? LOW : HIGH);
        lastTelemetryMS = millis();
    }

    static u32 lastBlinkTimeMS = 0;
    static bool state = false;
    if (millis() - lastBlinkTimeMS >= 0.5 * Time::SECOND)
    {
        state = !state;
        digitalWrite(BLINKING_PIN, state ? LOW : HIGH);
        lastBlinkTimeMS = millis();
    }
}
