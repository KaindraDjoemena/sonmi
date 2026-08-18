#include <Arduino.h>
#include <WiFi.h>
#include <HTTPClient.h>
#include "esp_camera.h"


using u32 = uint32_t;

// --- CONFIGURATION ---
#include "secrets.h"

const u32 PHOTO_INTERVAL_MS = 1000;

constexpr int PWDN_GPIO_NUM   =   32;
constexpr int RESET_GPIO_NUM  =  -1;
constexpr int XCLK_GPIO_NUM   =   0;
constexpr int SIOD_GPIO_NUM   =  26;
constexpr int SIOC_GPIO_NUM   =  27;

constexpr int Y9_GPIO_NUM     =  35;
constexpr int Y8_GPIO_NUM     =  34;
constexpr int Y7_GPIO_NUM     =  39;
constexpr int Y6_GPIO_NUM     =  36;
constexpr int Y5_GPIO_NUM     =  21;
constexpr int Y4_GPIO_NUM     =  19;
constexpr int Y3_GPIO_NUM     =  18;
constexpr int Y2_GPIO_NUM     =   5;
constexpr int VSYNC_GPIO_NUM  =  25;
constexpr int HREF_GPIO_NUM   =  23;
constexpr int PCLK_GPIO_NUM   =  22;

void setupWiFi()
{
    delay(10);
    Serial.printf("\nConnecting to %s\n", WIFI_SSID);
    
    WiFi.disconnect(true);
    delay(1000);
    
    WiFi.mode(WIFI_STA);
    
    IPAddress local_IP(192, 168, 4, 11);
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

void setupCamera()
{
    camera_config_t config;
    config.ledc_channel = LEDC_CHANNEL_0;
    config.ledc_timer   = LEDC_TIMER_0;
    config.pin_d0       = Y2_GPIO_NUM;
    config.pin_d1       = Y3_GPIO_NUM;
    config.pin_d2       = Y4_GPIO_NUM;
    config.pin_d3       = Y5_GPIO_NUM;
    config.pin_d4       = Y6_GPIO_NUM;
    config.pin_d5       = Y7_GPIO_NUM;
    config.pin_d6       = Y8_GPIO_NUM;
    config.pin_d7       = Y9_GPIO_NUM;
    config.pin_xclk     = XCLK_GPIO_NUM;
    config.pin_pclk     = PCLK_GPIO_NUM;
    config.pin_vsync    = VSYNC_GPIO_NUM;
    config.pin_href     = HREF_GPIO_NUM;
    config.pin_sccb_sda = SIOD_GPIO_NUM;
    config.pin_sccb_scl = SIOC_GPIO_NUM;
    config.pin_pwdn     = PWDN_GPIO_NUM;
    config.pin_reset    = RESET_GPIO_NUM;
    config.xclk_freq_hz = 20000000;
    config.pixel_format = PIXFORMAT_JPEG;
    config.grab_mode    = CAMERA_GRAB_WHEN_EMPTY;
    config.fb_location  = CAMERA_FB_IN_PSRAM;

    if (psramFound())
    {
        config.frame_size = FRAMESIZE_SVGA;
        config.jpeg_quality = 12;
        config.fb_count = 2;
    }
    else
    {
        config.frame_size = FRAMESIZE_SVGA;
        config.jpeg_quality = 15;
        config.fb_count = 1;
    }

    esp_err_t err = esp_camera_init(&config);
    if (err != ESP_OK)
    {
        Serial.printf("Camera init failed with error 0x%x\n", err);
        return;
    }
    Serial.println("Camera initialized.");
}

void uploadPhoto()
{
    camera_fb_t * fb = esp_camera_fb_get();
    if (!fb)
    {
        Serial.println("Camera capture failed");
        return;
    }

    Serial.printf("Captured frame: %u bytes\n", fb->len);

    if (WiFi.status() == WL_CONNECTED)
    {
        HTTPClient http;
        http.setReuse(false);
        http.setTimeout(3000);
        
        Serial.printf("Uploading %u bytes to %s\n", fb->len, SERVER_URL);
        http.begin(SERVER_URL);
        http.addHeader("Content-Type", "image/jpeg");
        http.addHeader("Authentication-Key", MEDIA_AUTH_KEY);

        int httpResponseCode = http.POST(fb->buf, fb->len);

        if (httpResponseCode > 0)
        {
            Serial.printf("HTTP POST Success, Code: %d\n", httpResponseCode);
            Serial.printf("Server Response: %s\n", http.getString().c_str());
        }
        else
        {
            Serial.printf("HTTP POST Error: %s\n", http.errorToString(httpResponseCode).c_str());
        }
        
        http.end();
    }
    else
    {
        Serial.println("WiFi Disconnected, skipping upload.");
    }

    esp_camera_fb_return(fb);
}

void setup()
{
    Serial.begin(115200);
    setupCamera();
    setupWiFi();
}

void loop()
{
    if (WiFi.status() != WL_CONNECTED)
    {
        Serial.println("WiFi connection lost! Reconnecting...");
        setupWiFi();
    }

    uploadPhoto();
    
    delay(PHOTO_INTERVAL_MS);
}
