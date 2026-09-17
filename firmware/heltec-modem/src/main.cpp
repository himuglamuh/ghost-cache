#include <Arduino.h>
#include <Preferences.h>
#include <RadioLib.h>
#include <SPI.h>
#include <SSD1306Wire.h>
#include <Wire.h>

namespace {
constexpr uint8_t PIN_NSS = 8, PIN_DIO1 = 14, PIN_RESET = 12, PIN_BUSY = 13;
constexpr uint8_t PIN_SCK = 9, PIN_MISO = 11, PIN_MOSI = 10;
constexpr uint8_t PIN_OLED_SDA = 17, PIN_OLED_SCL = 18, PIN_OLED_RESET = 21;
constexpr uint8_t PIN_VEXT = 36, OLED_ADDRESS = 0x3c;
constexpr uint8_t PROTOCOL_VERSION = 1;
constexpr size_t MAX_FRAME_PAYLOAD = 512, MAX_RAW_FRAME = MAX_FRAME_PAYLOAD + 12;
constexpr size_t MAX_ENCODED_FRAME = MAX_RAW_FRAME + (MAX_RAW_FRAME / 254) + 1;
constexpr uint32_t TX_TIMEOUT_MS = 10000, OLED_EVENT_MS = 1800;

enum MessageType : uint8_t {
  INFO_REQUEST = 0x01, TX_REQUEST = 0x02, SET_CONFIG = 0x03, SET_OLED = 0x04,
  RX_PACKET = 0x81, TX_RESULT = 0x82, STATUS = 0x83, ERROR_MESSAGE = 0x84,
};
enum OLEDMode : uint8_t { DIAGNOSTIC = 0, MINIMAL = 1, OFF = 2 };
enum RadioState : uint8_t { RADIO_FAILED, RADIO_RECEIVING, RADIO_TRANSMITTING };

struct RadioConfig {
  float frequencyMHz = 915.0f;
  float bandwidthKHz = 125.0f;
  uint8_t spreadingFactor = 7;
  uint8_t codingRate = 5;
  int8_t txPowerDBm = 5;
  uint16_t preambleSymbols = 8;
  uint8_t syncWord = 0x12;
};

SX1262 radio = new Module(PIN_NSS, PIN_DIO1, PIN_RESET, PIN_BUSY);
SSD1306Wire display(OLED_ADDRESS, -1, -1, GEOMETRY_128_64, I2C_ONE, 400000);
Preferences preferences;
RadioConfig config;
OLEDMode oledMode = DIAGNOSTIC;
RadioState radioState = RADIO_FAILED;
bool oledReady = false, radioInterruptAttached = false;
int16_t radioError = RADIOLIB_ERR_NONE;
const char* oledError = "not initialized";
volatile bool radioInterrupt = false;
uint8_t serialFrame[MAX_ENCODED_FRAME];
size_t serialFrameLength = 0;
bool serialDiscarding = false;
uint16_t txSequence = 0;
uint32_t txStartedAt = 0, oledEventUntil = 0;

void ARDUINO_ISR_ATTR onRadioInterrupt() { radioInterrupt = true; }

void bootDiagnostic(const char* component, const char* result) {
  Serial.print(component); Serial.print(": "); Serial.println(result);
}

uint16_t readU16(const uint8_t* p) { return uint16_t(p[0]) | (uint16_t(p[1]) << 8); }
void writeU16(uint8_t* p, uint16_t value) { p[0] = value; p[1] = value >> 8; }
void writeU32(uint8_t* p, uint32_t value) {
  p[0] = value; p[1] = value >> 8; p[2] = value >> 16; p[3] = value >> 24;
}
uint32_t readU32(const uint8_t* p) {
  return uint32_t(p[0]) | (uint32_t(p[1]) << 8) | (uint32_t(p[2]) << 16) | (uint32_t(p[3]) << 24);
}
float readFloat(const uint8_t* p) { uint32_t bits = readU32(p); float value; memcpy(&value, &bits, 4); return value; }
void writeFloat(uint8_t* p, float value) { uint32_t bits; memcpy(&bits, &value, 4); writeU32(p, bits); }

uint32_t crc32(const uint8_t* data, size_t length) {
  uint32_t crc = 0xffffffff;
  while (length--) {
    crc ^= *data++;
    for (uint8_t bit = 0; bit < 8; ++bit) crc = (crc >> 1) ^ (0xedb88320 & (-(int32_t)(crc & 1)));
  }
  return ~crc;
}

size_t cobsEncode(const uint8_t* input, size_t length, uint8_t* output) {
  size_t readIndex = 0, writeIndex = 1, codeIndex = 0;
  uint8_t code = 1;
  while (readIndex < length) {
    if (input[readIndex] == 0) {
      output[codeIndex] = code; code = 1; codeIndex = writeIndex++;
    } else {
      output[writeIndex++] = input[readIndex];
      if (++code == 0xff) { output[codeIndex] = code; code = 1; codeIndex = writeIndex++; }
    }
    ++readIndex;
  }
  output[codeIndex] = code;
  return writeIndex;
}

bool cobsDecode(const uint8_t* input, size_t length, uint8_t* output, size_t outputCapacity, size_t& outputLength) {
  size_t readIndex = 0, writeIndex = 0;
  while (readIndex < length) {
    uint8_t code = input[readIndex++];
    if (code == 0 || readIndex + code - 1 > length) return false;
    for (uint8_t i = 1; i < code; ++i) {
      if (writeIndex >= outputCapacity) return false;
      output[writeIndex++] = input[readIndex++];
    }
    if (code != 0xff && readIndex < length) {
      if (writeIndex >= outputCapacity) return false;
      output[writeIndex++] = 0;
    }
  }
  outputLength = writeIndex;
  return true;
}

void sendFrame(MessageType type, uint16_t sequence, const uint8_t* payload, uint16_t payloadLength) {
  if (payloadLength > MAX_FRAME_PAYLOAD) return;
  uint8_t raw[MAX_RAW_FRAME], encoded[MAX_RAW_FRAME + 8];
  raw[0] = 'G'; raw[1] = 'C'; raw[2] = PROTOCOL_VERSION; raw[3] = type;
  writeU16(raw + 4, sequence); writeU16(raw + 6, payloadLength);
  if (payloadLength) memcpy(raw + 8, payload, payloadLength);
  writeU32(raw + 8 + payloadLength, crc32(raw, 8 + payloadLength));
  size_t encodedLength = cobsEncode(raw, 12 + payloadLength, encoded);
  Serial.write(encoded, encodedLength); Serial.write(uint8_t(0));
}

void sendText(MessageType type, uint16_t sequence, int16_t code, const char* text) {
  uint8_t payload[192];
  size_t textLength = min(strlen(text), sizeof(payload) - 2);
  writeU16(payload, uint16_t(code)); memcpy(payload + 2, text, textLength);
  sendFrame(type, sequence, payload, textLength + 2);
}

String configLine() {
  return "SF" + String(config.spreadingFactor) + " BW" + String(int(config.bandwidthKHz)) + " CR" + String(config.codingRate);
}

void showOLED(const String& line1, const String& line2 = "") {
  if (!oledReady || oledMode == OFF) return;
  display.displayOn(); display.clear(); display.setTextAlignment(TEXT_ALIGN_LEFT);
  display.setFont(ArialMT_Plain_10); display.drawString(0, 0, oledMode == MINIMAL ? "GHOST" : "GHOST MODEM");
  if (oledMode == MINIMAL) display.drawString(0, 20, line1);
  else {
    display.drawString(0, 13, String(config.frequencyMHz, 3) + " MHz");
    display.drawString(0, 26, configLine()); display.drawString(0, 39, line1); display.drawString(0, 52, line2);
  }
  display.display();
}

void showReady() { showOLED(Serial ? "USB READY" : "USB WAIT", radioState == RADIO_RECEIVING ? "RX READY" : "RADIO ERROR"); }
void showEvent(const String& line1, const String& line2 = "") { showOLED(line1, line2); oledEventUntil = millis() + OLED_EVENT_MS; }

void applyOLEDMode(OLEDMode mode, bool persist) {
  oledMode = mode;
  if (persist && preferences.begin("ghost-modem", false)) { preferences.putUChar("oled", uint8_t(mode)); preferences.end(); }
  if (!oledReady) return;
  if (mode == OFF) { display.clear(); display.display(); display.displayOff(); } else showReady();
}

int16_t startReceive() {
  if (!radioInterruptAttached) {
    radio.setPacketReceivedAction(onRadioInterrupt);
    radioInterruptAttached = true;
  }
  radioInterrupt = false;
  int16_t state = radio.startReceive();
  radioState = state == RADIOLIB_ERR_NONE ? RADIO_RECEIVING : RADIO_FAILED;
  radioError = state;
  return state;
}

int16_t configureRadio() {
  if (radioError != RADIOLIB_ERR_NONE && !radioInterruptAttached) return radioError;
  int16_t state = radio.standby();
  if (state == RADIOLIB_ERR_NONE) state = radio.setFrequency(config.frequencyMHz);
  if (state == RADIOLIB_ERR_NONE) state = radio.setBandwidth(config.bandwidthKHz);
  if (state == RADIOLIB_ERR_NONE) state = radio.setSpreadingFactor(config.spreadingFactor);
  if (state == RADIOLIB_ERR_NONE) state = radio.setCodingRate(config.codingRate);
  if (state == RADIOLIB_ERR_NONE) state = radio.setOutputPower(config.txPowerDBm);
  if (state == RADIOLIB_ERR_NONE) state = radio.setPreambleLength(config.preambleSymbols);
  if (state == RADIOLIB_ERR_NONE) state = radio.setSyncWord(config.syncWord);
  if (state == RADIOLIB_ERR_NONE) state = startReceive();
  radioError = state;
  return state;
}

void sendStatus(uint16_t sequence) {
  char status[360];
  char radioStatus[48];
  if (radioState == RADIO_FAILED) snprintf(radioStatus, sizeof(radioStatus), "ERROR code=%d", radioError);
  else snprintf(radioStatus, sizeof(radioStatus), "%s", radioState == RADIO_TRANSMITTING ? "TX" : "OK RX_READY");
  snprintf(status, sizeof(status),
    "firmware: OK; serial: OK; radio: %s; oled: %s%s%s; protocol: v%u READY; frequency=%.3fMHz sf=%u bw=%.1fkHz cr=4/%u power=%ddBm preamble=%u sync=0x%02X oled_mode=%s",
    radioStatus, oledReady ? "OK" : "ERROR", oledReady ? "" : " ", oledReady ? "" : oledError, PROTOCOL_VERSION,
    config.frequencyMHz, config.spreadingFactor, config.bandwidthKHz, config.codingRate, config.txPowerDBm,
    config.preambleSymbols, config.syncWord, oledMode == DIAGNOSTIC ? "diagnostic" : (oledMode == MINIMAL ? "minimal" : "off"));
  sendFrame(STATUS, sequence, reinterpret_cast<uint8_t*>(status), strlen(status));
}

void handleCommand(const uint8_t* raw, size_t length) {
  if (length < 12 || raw[0] != 'G' || raw[1] != 'C') return;
  uint16_t sequence = readU16(raw + 4), payloadLength = readU16(raw + 6);
  if (raw[2] != PROTOCOL_VERSION) { sendText(ERROR_MESSAGE, sequence, -1, "unsupported protocol version"); return; }
  if (payloadLength > MAX_FRAME_PAYLOAD || length != size_t(12 + payloadLength) || readU32(raw + 8 + payloadLength) != crc32(raw, 8 + payloadLength)) return;
  const uint8_t* payload = raw + 8;
  switch (raw[3]) {
    case INFO_REQUEST:
      if (payloadLength != 0) sendText(ERROR_MESSAGE, sequence, -2, "INFO payload must be empty"); else sendStatus(sequence);
      break;
    case TX_REQUEST: {
      if (payloadLength > 255) { sendText(ERROR_MESSAGE, sequence, -3, "radio payload exceeds 255 bytes"); break; }
      if (radioState == RADIO_FAILED) { sendText(ERROR_MESSAGE, sequence, radioError, "radio is unavailable"); break; }
      if (radioState == RADIO_TRANSMITTING) { sendText(ERROR_MESSAGE, sequence, -4, "radio is busy transmitting"); break; }
      radioInterrupt = false;
      int16_t state = radio.startTransmit(payload, payloadLength);
      if (state != RADIOLIB_ERR_NONE) { sendText(TX_RESULT, sequence, state, "startTransmit failed"); startReceive(); }
      else { radioState = RADIO_TRANSMITTING; txSequence = sequence; txStartedAt = millis(); showEvent("TX " + String(payloadLength) + " bytes", "SENDING"); }
      break;
    }
    case SET_CONFIG:
      if (payloadLength != 15) { sendText(ERROR_MESSAGE, sequence, -5, "SET_CONFIG payload must be 15 bytes"); break; }
      if (radioState == RADIO_TRANSMITTING) { sendText(ERROR_MESSAGE, sequence, -4, "radio is busy transmitting"); break; }
      if (!radioInterruptAttached) { sendText(ERROR_MESSAGE, sequence, radioError, "radio is unavailable"); break; }
      if (payload[14] != 0) { sendText(ERROR_MESSAGE, sequence, -5, "SET_CONFIG reserved byte must be zero"); break; }
      { RadioConfig previous = config;
        config.frequencyMHz = readFloat(payload); config.bandwidthKHz = readFloat(payload + 4);
        config.spreadingFactor = payload[8]; config.codingRate = payload[9]; config.txPowerDBm = int8_t(payload[10]);
        config.preambleSymbols = readU16(payload + 11); config.syncWord = payload[13];
        int16_t state = configureRadio();
        if (state == RADIOLIB_ERR_NONE) sendStatus(sequence);
        else {
          config = previous;
          int16_t restoreState = configureRadio();
          if (restoreState != RADIOLIB_ERR_NONE) { radioState = RADIO_FAILED; radioError = restoreState; }
          sendText(ERROR_MESSAGE, sequence, state, "invalid or failed radio configuration");
        }
      }
      break;
    case SET_OLED:
      if (payloadLength != 1 || payload[0] > OFF) sendText(ERROR_MESSAGE, sequence, -6, "invalid OLED mode");
      else { applyOLEDMode(OLEDMode(payload[0]), true); sendStatus(sequence); }
      break;
    default: sendText(ERROR_MESSAGE, sequence, -7, "unknown message type");
  }
}

void handleSerial() {
  while (Serial.available()) {
    uint8_t b = Serial.read();
    if (b == 0) {
      if (!serialDiscarding && serialFrameLength) {
        uint8_t raw[MAX_RAW_FRAME]; size_t rawLength = 0;
        if (cobsDecode(serialFrame, serialFrameLength, raw, sizeof(raw), rawLength)) handleCommand(raw, rawLength);
      }
      serialFrameLength = 0; serialDiscarding = false;
    } else if (!serialDiscarding) {
      if (serialFrameLength >= sizeof(serialFrame)) { serialFrameLength = 0; serialDiscarding = true; }
      else serialFrame[serialFrameLength++] = b;
    }
  }
}

void handleRadio() {
  if (radioState == RADIO_TRANSMITTING && millis() - txStartedAt > TX_TIMEOUT_MS) {
    radio.finishTransmit(); sendText(TX_RESULT, txSequence, -8, "TX timeout"); startReceive(); showEvent("TX FAILED", "TIMEOUT"); return;
  }
  if (!radioInterrupt) return;
  radioInterrupt = false;
  if (radioState == RADIO_TRANSMITTING) {
    int16_t state = radio.finishTransmit();
    sendText(TX_RESULT, txSequence, state, state == RADIOLIB_ERR_NONE ? "transmitted" : "finishTransmit failed");
    startReceive(); showEvent(state == RADIOLIB_ERR_NONE ? "TX OK" : "TX FAILED"); return;
  }
  if (radioState == RADIO_RECEIVING) {
    size_t length = radio.getPacketLength();
    if (length > 255) { startReceive(); return; }
    uint8_t packet[255]; int16_t state = radio.readData(packet, length);
    if (state == RADIOLIB_ERR_NONE) {
      uint8_t event[263]; float rssi = radio.getRSSI(), snr = radio.getSNR();
      writeFloat(event, rssi); writeFloat(event + 4, snr); memcpy(event + 8, packet, length);
      sendFrame(RX_PACKET, 0, event, length + 8);
      showEvent("RX " + String(length) + " bytes", String(rssi, 1) + "dBm " + String(snr, 1) + "dB");
    } else sendText(ERROR_MESSAGE, 0, state, "readData failed");
    startReceive();
  }
}
}  // namespace

void setup() {
  Serial.setRxBufferSize(MAX_ENCODED_FRAME + 1);
  Serial.begin(115200);
  delay(100);
  Serial.println("ghost-modem boot");
  bootDiagnostic("serial", "ok (UART0)");

  pinMode(PIN_VEXT, OUTPUT);
  digitalWrite(PIN_VEXT, LOW);  // Heltec V3 Vext is active-low and powers the OLED.
  delay(50);
  bootDiagnostic("vext", "enabled");

  pinMode(PIN_OLED_RESET, OUTPUT);
  digitalWrite(PIN_OLED_RESET, LOW); delay(20);
  digitalWrite(PIN_OLED_RESET, HIGH); delay(50);
  bool wireReady = Wire.begin(PIN_OLED_SDA, PIN_OLED_SCL, 400000);
  if (!wireReady) {
    oledError = "I2C initialization failed";
  } else {
    Wire.beginTransmission(OLED_ADDRESS);
    if (Wire.endTransmission() != 0) oledError = "SSD1306 not found at 0x3C";
    else if (!display.init()) oledError = "display buffer allocation failed";
    else { oledReady = true; oledError = ""; display.flipScreenVertically(); }
  }
  bootDiagnostic("oled", oledReady ? "ok" : oledError);

  // Opening read/write creates the namespace on a virgin board without the
  // expected read-only nvs_open NOT_FOUND diagnostic.
  if (preferences.begin("ghost-modem", false)) {
    uint8_t saved = preferences.getUChar("oled", DIAGNOSTIC);
    preferences.end();
    if (saved <= OFF) oledMode = OLEDMode(saved);
  } else bootDiagnostic("nvs", "unavailable; using diagnostic OLED mode");

  SPI.begin(PIN_SCK, PIN_MISO, PIN_MOSI, PIN_NSS);
  bootDiagnostic("spi", "ok");
  radio.tcxoVoltage = 1.6;
  int16_t state = radio.begin(config.frequencyMHz, config.bandwidthKHz, config.spreadingFactor, config.codingRate,
                              config.syncWord, config.txPowerDBm, config.preambleSymbols, 1.6, false);
  radioError = state;
  if (state == RADIOLIB_ERR_NONE) {
    // Install DIO1 only after RadioLib has initialized the radio. startReceive
    // must not detach an action before Arduino has installed its ISR service.
    radio.setPacketReceivedAction(onRadioInterrupt);
    radioInterruptAttached = true;
    radioInterrupt = false;
    state = radio.startReceive();
    radioError = state;
  }
  radioState = state == RADIOLIB_ERR_NONE ? RADIO_RECEIVING : RADIO_FAILED;
  if (radioState == RADIO_RECEIVING) bootDiagnostic("radio", "ok");
  else { char detail[48]; snprintf(detail, sizeof(detail), "ERROR code=%d", radioError); bootDiagnostic("radio", detail); }
  applyOLEDMode(oledMode, false);
  bootDiagnostic("protocol", "ready");
  // Terminate development text as a malformed serial frame boundary so the
  // first subsequent COBS request is decoded independently.
  Serial.write(uint8_t(0));
  Serial.flush();
}

void loop() {
  handleSerial(); handleRadio();
  if (oledMode != OFF && oledEventUntil && int32_t(millis() - oledEventUntil) >= 0) { oledEventUntil = 0; showReady(); }
  delay(1);
}
