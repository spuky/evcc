package modbus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/grid-x/modbus"
	"github.com/volkszaehler/mbmd/meters"
)

const (
	// SolarmanV5 protocol constants
	solarmanStart       = 0xA5
	solarmanEnd         = 0x15
	solarmanRequestCmd  = 0x4510
	solarmanResponseCmd = 0x1510
	solarmanPort        = 8899
	solarmanFrameType   = 0x02
	solarmanSensorType  = 0x0000

	// Packet sizes
	headerSize         = 11
	requestPayloadMin  = 15
	responsePayloadMin = 14
	trailerSize        = 2
	minPacketSize      = headerSize + requestPayloadMin + trailerSize
)

// SolarmanV5Header represents the SolarmanV5 packet header
type SolarmanV5Header struct {
	Start       uint8  // 0xA5
	Length      uint16 // Length of payload
	ControlCode uint16 // 0x4510 for requests, 0x1510 for responses
	Serial      uint16 // Sequence number
	LoggerSerial uint32 // Logger serial number
}

// SolarmanV5RequestPayload represents the request payload structure
type SolarmanV5RequestPayload struct {
	FrameType       uint8  // 0x02
	SensorType      uint16 // 0x0000
	TotalWorkingTime uint32 // 0x00000000
	PowerOnTime     uint32 // Frame power on time
	ModbusFrame     []byte // Modbus RTU frame
}

// SolarmanV5ResponsePayload represents the response payload structure
type SolarmanV5ResponsePayload struct {
	FrameType       uint8  // Frame type
	Status          uint8  // 0x01 for real-time data
	TotalWorkingTime uint32 // Total working time in seconds
	PowerOnTime     uint32 // Current uptime in seconds
	OffsetTime      uint32 // Offset timestamp
	ModbusFrame     []byte // Modbus RTU response frame
}

// SolarmanV5Trailer represents the packet trailer
type SolarmanV5Trailer struct {
	Checksum uint8 // V5 frame checksum
	End      uint8 // 0x15
}

// SolarmanV5Connection implements a connection to a SolarmanV5 data logger
type SolarmanV5Connection struct {
	address      string
	conn         net.Conn
	loggerSerial uint32
	serial       uint16
	timeout      time.Duration
	mutex        sync.Mutex
	logger       meters.Logger
	slaveID      uint8
}

// NewSolarmanV5Connection creates a new SolarmanV5 connection
func NewSolarmanV5Connection(address string, loggerSerial uint32) (*SolarmanV5Connection, error) {
	if address == "" {
		return nil, fmt.Errorf("address cannot be empty")
	}

	// Add default port if not specified
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = fmt.Sprintf("%s:%d", address, solarmanPort)
	}

	return &SolarmanV5Connection{
		address:      address,
		loggerSerial: loggerSerial,
		serial:       1,
		timeout:      5 * time.Second,
		slaveID:      1, // Default slave ID
	}, nil
}

// Connect establishes the TCP connection
func (c *SolarmanV5Connection) Connect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn != nil {
		return nil
	}

	conn, err := net.DialTimeout("tcp", c.address, c.timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to SolarmanV5 logger: %w", err)
	}

	c.conn = conn
	return nil
}

// Close closes the connection
func (c *SolarmanV5Connection) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// String returns the connection string representation
func (c *SolarmanV5Connection) String() string {
	return c.address
}

// Logger sets the logger for this connection
func (c *SolarmanV5Connection) Logger(logger meters.Logger) {
	c.logger = logger
}

// Timeout sets the connection timeout
func (c *SolarmanV5Connection) Timeout(timeout time.Duration) time.Duration {
	oldTimeout := c.timeout
	c.timeout = timeout
	return oldTimeout
}

// ConnectDelay is a no-op for SolarmanV5
func (c *SolarmanV5Connection) ConnectDelay(delay time.Duration) {
	// No-op
}

// Slave sets the modbus device id
func (c *SolarmanV5Connection) Slave(deviceID uint8) {
	c.slaveID = deviceID
}

// SetDeadline sets read/write deadlines
func (c *SolarmanV5Connection) SetDeadline() error {
	if c.conn == nil {
		return fmt.Errorf("connection not established")
	}

	deadline := time.Now().Add(c.timeout)
	if err := c.conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("failed to set deadline: %w", err)
	}

	return nil
}

// SendModbusFrame sends a Modbus RTU frame encapsulated in SolarmanV5 protocol
func (c *SolarmanV5Connection) SendModbusFrame(modbusFrame []byte) ([]byte, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err := c.Connect(); err != nil {
		return nil, err
	}

	if err := c.SetDeadline(); err != nil {
		return nil, err
	}

	// Build request packet
	packet, err := c.buildRequestPacket(modbusFrame)
	if err != nil {
		return nil, fmt.Errorf("failed to build request packet: %w", err)
	}

	// Send packet
	if c.logger != nil {
		c.logger.Printf("solarmanv5 tx: % x", packet)
	}

	if _, err := c.conn.Write(packet); err != nil {
		c.Close()
		return nil, fmt.Errorf("failed to send packet: %w", err)
	}

	// Read response
	response, err := c.readResponse()
	if err != nil {
		c.Close()
		return nil, err
	}

	if c.logger != nil {
		c.logger.Printf("solarmanv5 rx: % x", response)
	}

	// Parse response and extract Modbus frame
	modbusResponse, err := c.parseResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Handle double CRC issue (some DEYE inverters)
	modbusResponse = c.fixDoubleCRC(modbusResponse)

	return modbusResponse, nil
}

// buildRequestPacket builds a SolarmanV5 request packet
func (c *SolarmanV5Connection) buildRequestPacket(modbusFrame []byte) ([]byte, error) {
	payloadSize := requestPayloadMin + len(modbusFrame)
	totalSize := headerSize + payloadSize + trailerSize

	buf := bytes.NewBuffer(make([]byte, 0, totalSize))

	// Header
	header := SolarmanV5Header{
		Start:       solarmanStart,
		Length:      uint16(payloadSize),
		ControlCode: solarmanRequestCmd,
		Serial:      c.serial,
		LoggerSerial: c.loggerSerial,
	}

	c.serial++ // Increment sequence number

	// Write header (little endian except start byte)
	buf.WriteByte(header.Start)
	binary.Write(buf, binary.LittleEndian, header.Length)
	binary.Write(buf, binary.LittleEndian, header.ControlCode)
	binary.Write(buf, binary.LittleEndian, header.Serial)
	binary.Write(buf, binary.LittleEndian, header.LoggerSerial)

	// Payload
	payload := SolarmanV5RequestPayload{
		FrameType:       solarmanFrameType,
		SensorType:      solarmanSensorType,
		TotalWorkingTime: 0,
		PowerOnTime:     uint32(time.Now().Unix()),
		ModbusFrame:     modbusFrame,
	}

	// Write payload (little endian)
	buf.WriteByte(payload.FrameType)
	binary.Write(buf, binary.LittleEndian, payload.SensorType)
	binary.Write(buf, binary.LittleEndian, payload.TotalWorkingTime)
	binary.Write(buf, binary.LittleEndian, payload.PowerOnTime)
	buf.Write(payload.ModbusFrame)

	// Calculate checksum (exclude start byte and checksum itself)
	data := buf.Bytes()[1:]
	checksum := c.calculateChecksum(data)

	// Write trailer
	buf.WriteByte(checksum)
	buf.WriteByte(solarmanEnd)

	return buf.Bytes(), nil
}

// readResponse reads the response from the connection
func (c *SolarmanV5Connection) readResponse() ([]byte, error) {
	// First, read the header to determine payload length
	headerBuf := make([]byte, headerSize)
	if _, err := c.conn.Read(headerBuf); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Parse header to get payload length
	if headerBuf[0] != solarmanStart {
		return nil, fmt.Errorf("invalid start byte: expected %02x, got %02x", solarmanStart, headerBuf[0])
	}

	payloadLength := binary.LittleEndian.Uint16(headerBuf[1:3])

	// Read payload and trailer
	remainingBuf := make([]byte, int(payloadLength)+trailerSize)
	if _, err := c.conn.Read(remainingBuf); err != nil {
		return nil, fmt.Errorf("failed to read payload and trailer: %w", err)
	}

	// Combine header + payload + trailer
	fullResponse := append(headerBuf, remainingBuf...)

	// Verify checksum
	checksumIndex := len(fullResponse) - 2
	expectedChecksum := fullResponse[checksumIndex]
	actualChecksum := c.calculateChecksum(fullResponse[1:checksumIndex])

	if actualChecksum != expectedChecksum {
		return nil, fmt.Errorf("checksum mismatch: expected %02x, got %02x", expectedChecksum, actualChecksum)
	}

	// Verify end byte
	if fullResponse[len(fullResponse)-1] != solarmanEnd {
		return nil, fmt.Errorf("invalid end byte: expected %02x, got %02x", solarmanEnd, fullResponse[len(fullResponse)-1])
	}

	return fullResponse, nil
}

// parseResponse parses the SolarmanV5 response and extracts the Modbus frame
func (c *SolarmanV5Connection) parseResponse(response []byte) ([]byte, error) {
	if len(response) < minPacketSize {
		return nil, fmt.Errorf("response too short: %d bytes", len(response))
	}

	// Skip header (11 bytes)
	payload := response[headerSize : len(response)-trailerSize]

	if len(payload) < responsePayloadMin {
		return nil, fmt.Errorf("payload too short: %d bytes", len(payload))
	}

	// Parse response payload
	frameType := payload[0]
	status := payload[1]

	if frameType != solarmanFrameType {
		return nil, fmt.Errorf("unexpected frame type: %02x", frameType)
	}

	if status != 0x01 {
		return nil, fmt.Errorf("unexpected status: %02x", status)
	}

	// Extract Modbus frame (starts at byte 14 of payload)
	modbusFrame := payload[responsePayloadMin:]

	if len(modbusFrame) == 0 {
		return nil, fmt.Errorf("empty Modbus frame in response")
	}

	return modbusFrame, nil
}

// calculateChecksum calculates the SolarmanV5 checksum
func (c *SolarmanV5Connection) calculateChecksum(data []byte) uint8 {
	var sum uint32
	for _, b := range data {
		sum += uint32(b)
	}
	return uint8(sum & 0xFF)
}

// fixDoubleCRC handles the double CRC issue with some inverters
func (c *SolarmanV5Connection) fixDoubleCRC(modbusFrame []byte) []byte {
	if len(modbusFrame) < 4 {
		return modbusFrame
	}

	// Check if we have a double CRC by verifying the original CRC
	// and then checking if removing the last 2 bytes gives a valid CRC
	originalLen := len(modbusFrame)

	// Calculate CRC for the frame without the last 2 bytes
	frameWithoutLastCRC := modbusFrame[:originalLen-2]
	if len(frameWithoutLastCRC) < 3 {
		return modbusFrame
	}

	// Calculate CRC16 for Modbus RTU
	expectedCRC := crc16(frameWithoutLastCRC)
	actualCRC := binary.LittleEndian.Uint16(frameWithoutLastCRC[len(frameWithoutLastCRC)-2:])

	if expectedCRC == actualCRC {
		// The frame without the last 2 bytes has a valid CRC, so remove the double CRC
		return frameWithoutLastCRC
	}

	return modbusFrame
}

// crc16 calculates CRC16 for Modbus RTU
func crc16(data []byte) uint16 {
	const poly = 0xA001
	crc := uint16(0xFFFF)

	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}
	}

	return crc
}

// ModbusClient returns a Modbus client interface
func (c *SolarmanV5Connection) ModbusClient() modbus.Client {
	return &SolarmanV5ModbusClient{conn: c}
}

// SolarmanV5ModbusClient wraps SolarmanV5Connection to provide Modbus client interface
type SolarmanV5ModbusClient struct {
	conn *SolarmanV5Connection
}

// ReadCoils implements Modbus function code 01
func (mc *SolarmanV5ModbusClient) ReadCoils(address, quantity uint16) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.ReadCoils(address, quantity)
}

// ReadDiscreteInputs implements Modbus function code 02
func (mc *SolarmanV5ModbusClient) ReadDiscreteInputs(address, quantity uint16) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.ReadDiscreteInputs(address, quantity)
}

// ReadHoldingRegisters implements Modbus function code 03
func (mc *SolarmanV5ModbusClient) ReadHoldingRegisters(address, quantity uint16) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.ReadHoldingRegisters(address, quantity)
}

// ReadInputRegisters implements Modbus function code 04
func (mc *SolarmanV5ModbusClient) ReadInputRegisters(address, quantity uint16) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.ReadInputRegisters(address, quantity)
}

// WriteSingleCoil implements Modbus function code 05
func (mc *SolarmanV5ModbusClient) WriteSingleCoil(address, value uint16) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.WriteSingleCoil(address, value)
}

// WriteSingleRegister implements Modbus function code 06
func (mc *SolarmanV5ModbusClient) WriteSingleRegister(address, value uint16) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.WriteSingleRegister(address, value)
}

// WriteMultipleCoils implements Modbus function code 15
func (mc *SolarmanV5ModbusClient) WriteMultipleCoils(address, quantity uint16, value []byte) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.WriteMultipleCoils(address, quantity, value)
}

// WriteMultipleRegisters implements Modbus function code 16
func (mc *SolarmanV5ModbusClient) WriteMultipleRegisters(address, quantity uint16, value []byte) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.WriteMultipleRegisters(address, quantity, value)
}

// ReadWriteMultipleRegisters implements Modbus function code 23
func (mc *SolarmanV5ModbusClient) ReadWriteMultipleRegisters(readAddress, readQuantity, writeAddress, writeQuantity uint16, value []byte) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.ReadWriteMultipleRegisters(readAddress, readQuantity, writeAddress, writeQuantity, value)
}

// MaskWriteRegister implements Modbus function code 22
func (mc *SolarmanV5ModbusClient) MaskWriteRegister(address, andMask, orMask uint16) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.MaskWriteRegister(address, andMask, orMask)
}

// ReadFIFOQueue implements Modbus function code 24
func (mc *SolarmanV5ModbusClient) ReadFIFOQueue(address uint16) ([]byte, error) {
	client := &SolarmanV5Client{conn: mc.conn, slaveID: mc.conn.slaveID}
	return client.ReadFIFOQueue(address)
}

// Clone creates a copy of the connection with a different slave ID
// For SolarmanV5, the slave ID is part of the Modbus frame
func (c *SolarmanV5Connection) Clone(slaveID uint8) meters.Connection {
	return &SolarmanV5Connection{
		address:      c.address,
		conn:         nil, // New connection will be established when needed
		loggerSerial: c.loggerSerial,
		serial:       c.serial,
		timeout:      c.timeout,
		slaveID:      slaveID,
	}
}