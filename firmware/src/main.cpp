#include <Arduino.h>
#include <DHT.h>

#include <array>

using u8  = uint8_t;
using u32 = uint32_t;

enum class RelayPins : u8 {
    GROW_LIGHT  = 26,
    WATER_PUMP  = 27,
    INTAKE_FAN  = 32,
    EXHAUST_FAN = 33
};

enum class SensorPins : u8 {
    TEMP_AIR_HUMIDITY = 4,
    SOIL_HUMIDITY     = 34
};

constexpr int BLINKING_PIN = 12;

namespace Time {
    constexpr u32 SECOND = 1000;
    constexpr u32 MINUTE = 60 * SECOND;
    constexpr u32 HOUR   = 60 * MINUTE;
    constexpr u32 DAY    = 24 * HOUR;
}

constexpr u8 numRelays = 4;
constexpr std::array<RelayPins, numRelays> relays = { RelayPins::GROW_LIGHT, RelayPins::WATER_PUMP, RelayPins::INTAKE_FAN, RelayPins::EXHAUST_FAN };

constexpr u8 numSensors = 1;
constexpr std::array<SensorPins, numSensors> sensors = { SensorPins::SOIL_HUMIDITY };


class SonDHT22
{
public:
    SonDHT22(SensorPins pin)
    : _dht(static_cast<u8>(pin), DHT22)
    {
    }

    ~SonDHT22()
    {
    }

    // Move and Copying

    void Init()
    {
        _dht.begin();
    }

    bool Read(u32 delayS = 2)
    {
        u32 currMillis = millis();

        if (currMillis - _lastReadTimeMS >= _readInterval)
        {
            _reading.tempC    = _dht.readTemperature();
            _reading.humidity = _dht.readHumidity();

            _lastReadTimeMS += _readInterval;  // Prevent tick drifting

            return true;
        }

        return false;
    }

    void Log(HardwareSerial &s)
    {
        if (isnan(_reading.tempC) || isnan(_reading.humidity))
        {
            s.println("failed to read from dht sensor");
        }
        else
        {
            s.printf("temp: %.2fc, humidity: %.2f%%\n", _reading.tempC, _reading.humidity);
        }
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

SonDHT22 dht(SensorPins::TEMP_AIR_HUMIDITY);

///////////////////////////////////////////////////////////////////////////////////
void setup()
{
    Serial.begin(9600);

    dht.Init();

    delay(1 * Time::SECOND);

    Serial.println("\n--- Sonmi Relay Test Controller ---");
    Serial.println("Initializing pins...");

    pinMode(static_cast<u8>(BLINKING_PIN), OUTPUT);

    for (u8 i = 0; i < numRelays; i++)
    {
        digitalWrite(static_cast<u8>(relays[i]), HIGH);  // initialize relays to high (inactive)
        pinMode(static_cast<u8>(relays[i]), OUTPUT);
    }

    for (u8 i = 0; i < numSensors; i++)
    {
        pinMode(static_cast<u8>(sensors[i]), INPUT);
    }
}

void loop()
{
    if (dht.Read())
        dht.Log(Serial);

    static u32 lastBlinkTimeMS = 0;
    static bool state = false;
    if (millis() - lastBlinkTimeMS >= 0.5 * Time::SECOND)
    {
        state = !state;

        digitalWrite(static_cast<u8>(BLINKING_PIN), state ? LOW : HIGH);

        lastBlinkTimeMS = millis();
    }
}