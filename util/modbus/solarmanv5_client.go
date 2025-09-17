package modbus

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/grid-x/modbus"
	"github.com/volkszaehler/mbmd/meters"
)

// SolarmanV5Client implements Modbus client operations over SolarmanV5 protocol
type SolarmanV5Client struct {
	conn    *SolarmanV5Connection
	slaveID uint8
}

// NewSolarmanV5Client creates a new Modbus client using SolarmanV5 protocol
func NewSolarmanV5Client(address string, loggerSerial uint32, slaveID uint8) (*SolarmanV5Client, error) {
	conn, err := NewSolarmanV5Connection(address, loggerSerial)
	if err != nil {
		return nil, err
	}

	return &SolarmanV5Client{
		conn:    conn,
		slaveID: slaveID,
	}, nil
}

// String returns the client connection string
func (c *SolarmanV5Client) String() string {
	return fmt.Sprintf("solarmanv5://%s/%d", c.conn.String(), c.slaveID)
}

// Logger sets the logger for this client
func (c *SolarmanV5Client) Logger(logger meters.Logger) {
	c.conn.Logger(logger)
}

// Timeout sets the connection timeout
func (c *SolarmanV5Client) Timeout(timeout time.Duration) time.Duration {
	return c.conn.Timeout(timeout)
}

// ConnectDelay is a no-op for SolarmanV5
func (c *SolarmanV5Client) ConnectDelay(delay time.Duration) {
	// No-op
}

// Slave sets the slave ID
func (c *SolarmanV5Client) Slave(slaveID uint8) {
	c.slaveID = slaveID
}

// Close closes the connection
func (c *SolarmanV5Client) Close() {
	c.conn.Close()
}

// Clone creates a copy of the client with a different slave ID
func (c *SolarmanV5Client) Clone(slaveID uint8) meters.Connection {
	clonedConn := c.conn.Clone(slaveID).(*SolarmanV5Connection)
	return &SolarmanV5Client{
		conn:    clonedConn,
		slaveID: slaveID,
	}
}

// ModbusClient returns the client itself (compatibility with existing interface)
func (c *SolarmanV5Client) ModbusClient() modbus.Client {
	return c
}

// ReadCoils reads coil status (function code 01)
func (c *SolarmanV5Client) ReadCoils(address, quantity uint16) ([]byte, error) {
	if quantity < 1 || quantity > 2000 {
		return nil, fmt.Errorf("invalid quantity: %d (must be 1-2000)", quantity)
	}

	request := c.buildModbusRequest(0x01, address, quantity, nil)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x01)
}

// WriteSingleCoil writes a single coil (function code 05)
func (c *SolarmanV5Client) WriteSingleCoil(address, value uint16) ([]byte, error) {
	if value != 0x0000 && value != 0xFF00 {
		return nil, fmt.Errorf("invalid coil value: %04x (must be 0x0000 or 0xFF00)", value)
	}

	request := c.buildModbusRequest(0x05, address, value, nil)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x05)
}

// ReadDiscreteInputs reads discrete input status (function code 02)
func (c *SolarmanV5Client) ReadDiscreteInputs(address, quantity uint16) ([]byte, error) {
	if quantity < 1 || quantity > 2000 {
		return nil, fmt.Errorf("invalid quantity: %d (must be 1-2000)", quantity)
	}

	request := c.buildModbusRequest(0x02, address, quantity, nil)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x02)
}

// ReadInputRegisters reads input registers (function code 04)
func (c *SolarmanV5Client) ReadInputRegisters(address, quantity uint16) ([]byte, error) {
	if quantity < 1 || quantity > 125 {
		return nil, fmt.Errorf("invalid quantity: %d (must be 1-125)", quantity)
	}

	request := c.buildModbusRequest(0x04, address, quantity, nil)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x04)
}

// ReadHoldingRegisters reads holding registers (function code 03)
func (c *SolarmanV5Client) ReadHoldingRegisters(address, quantity uint16) ([]byte, error) {
	if quantity < 1 || quantity > 125 {
		return nil, fmt.Errorf("invalid quantity: %d (must be 1-125)", quantity)
	}

	request := c.buildModbusRequest(0x03, address, quantity, nil)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x03)
}

// WriteSingleRegister writes a single register (function code 06)
func (c *SolarmanV5Client) WriteSingleRegister(address, value uint16) ([]byte, error) {
	request := c.buildModbusRequest(0x06, address, value, nil)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x06)
}

// WriteMultipleCoils writes multiple coils (function code 15)
func (c *SolarmanV5Client) WriteMultipleCoils(address, quantity uint16, value []byte) ([]byte, error) {
	if quantity < 1 || quantity > 1968 {
		return nil, fmt.Errorf("invalid quantity: %d (must be 1-1968)", quantity)
	}

	expectedBytes := (int(quantity) + 7) / 8
	if len(value) != expectedBytes {
		return nil, fmt.Errorf("invalid value length: got %d, expected %d bytes", len(value), expectedBytes)
	}

	request := c.buildModbusRequest(0x0F, address, quantity, value)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x0F)
}

// WriteMultipleRegisters writes multiple registers (function code 16)
func (c *SolarmanV5Client) WriteMultipleRegisters(address, quantity uint16, value []byte) ([]byte, error) {
	if quantity < 1 || quantity > 123 {
		return nil, fmt.Errorf("invalid quantity: %d (must be 1-123)", quantity)
	}

	expectedBytes := int(quantity) * 2
	if len(value) != expectedBytes {
		return nil, fmt.Errorf("invalid value length: got %d, expected %d bytes", len(value), expectedBytes)
	}

	request := c.buildModbusRequest(0x10, address, quantity, value)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x10)
}

// MaskWriteRegister modifies a register using AND and OR masks (function code 22)
func (c *SolarmanV5Client) MaskWriteRegister(address, andMask, orMask uint16) ([]byte, error) {
	// Build custom request for mask write register
	data := make([]byte, 6)
	binary.BigEndian.PutUint16(data[0:2], andMask)
	binary.BigEndian.PutUint16(data[2:4], orMask)

	request := c.buildModbusRequest(0x16, address, 0, data)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x16)
}

// ReadWriteMultipleRegisters reads and writes multiple registers in one operation (function code 23)
func (c *SolarmanV5Client) ReadWriteMultipleRegisters(readAddress, readQuantity, writeAddress, writeQuantity uint16, value []byte) ([]byte, error) {
	if readQuantity < 1 || readQuantity > 125 {
		return nil, fmt.Errorf("invalid read quantity: %d (must be 1-125)", readQuantity)
	}

	if writeQuantity < 1 || writeQuantity > 121 {
		return nil, fmt.Errorf("invalid write quantity: %d (must be 1-121)", writeQuantity)
	}

	expectedBytes := int(writeQuantity) * 2
	if len(value) != expectedBytes {
		return nil, fmt.Errorf("invalid value length: got %d, expected %d bytes", len(value), expectedBytes)
	}

	// Build custom request for read/write multiple registers
	data := make([]byte, 5+len(value))
	binary.BigEndian.PutUint16(data[0:2], writeAddress)
	binary.BigEndian.PutUint16(data[2:4], writeQuantity)
	data[4] = uint8(len(value))
	copy(data[5:], value)

	request := c.buildModbusRequest(0x17, readAddress, readQuantity, data)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x17)
}

// ReadFIFOQueue reads FIFO queue (function code 24)
func (c *SolarmanV5Client) ReadFIFOQueue(address uint16) ([]byte, error) {
	request := c.buildModbusRequest(0x18, address, 0, nil)
	response, err := c.conn.SendModbusFrame(request)
	if err != nil {
		return nil, err
	}

	return c.parseModbusResponse(response, 0x18)
}

// buildModbusRequest builds a Modbus RTU request frame
func (c *SolarmanV5Client) buildModbusRequest(functionCode uint8, address, quantity uint16, data []byte) []byte {
	var request []byte

	// Add slave ID and function code
	request = append(request, c.slaveID, functionCode)

	// Add address and quantity/value for most function codes
	switch functionCode {
	case 0x01, 0x02, 0x03, 0x04: // Read functions
		request = append(request, make([]byte, 4)...)
		binary.BigEndian.PutUint16(request[2:4], address)
		binary.BigEndian.PutUint16(request[4:6], quantity)

	case 0x05, 0x06: // Write single functions
		request = append(request, make([]byte, 4)...)
		binary.BigEndian.PutUint16(request[2:4], address)
		binary.BigEndian.PutUint16(request[4:6], quantity) // value for these functions

	case 0x0F, 0x10: // Write multiple functions
		request = append(request, make([]byte, 5)...)
		binary.BigEndian.PutUint16(request[2:4], address)
		binary.BigEndian.PutUint16(request[4:6], quantity)
		request[6] = uint8(len(data))
		request = append(request, data...)

	case 0x16: // Mask write register
		request = append(request, make([]byte, 2)...)
		binary.BigEndian.PutUint16(request[2:4], address)
		request = append(request, data...)

	case 0x17: // Read/write multiple registers
		request = append(request, make([]byte, 4)...)
		binary.BigEndian.PutUint16(request[2:4], address)
		binary.BigEndian.PutUint16(request[4:6], quantity)
		request = append(request, data...)

	case 0x18: // Read FIFO queue
		request = append(request, make([]byte, 2)...)
		binary.BigEndian.PutUint16(request[2:4], address)
	}

	// Calculate and append CRC16
	crc := crc16(request)
	request = append(request, make([]byte, 2)...)
	binary.LittleEndian.PutUint16(request[len(request)-2:], crc)

	return request
}

// parseModbusResponse parses a Modbus RTU response frame
func (c *SolarmanV5Client) parseModbusResponse(response []byte, expectedFunctionCode uint8) ([]byte, error) {
	if len(response) < 3 {
		return nil, fmt.Errorf("response too short: %d bytes", len(response))
	}

	// Check slave ID
	if response[0] != c.slaveID {
		return nil, fmt.Errorf("unexpected slave ID: expected %d, got %d", c.slaveID, response[0])
	}

	// Check for error response
	if response[1] == (expectedFunctionCode | 0x80) {
		if len(response) < 5 {
			return nil, fmt.Errorf("error response too short")
		}
		return nil, fmt.Errorf("modbus error: function code %02x, exception code %02x", expectedFunctionCode, response[2])
	}

	// Check function code
	if response[1] != expectedFunctionCode {
		return nil, fmt.Errorf("unexpected function code: expected %02x, got %02x", expectedFunctionCode, response[1])
	}

	// Verify CRC
	if len(response) >= 4 {
		frameWithoutCRC := response[:len(response)-2]
		expectedCRC := crc16(frameWithoutCRC)
		actualCRC := binary.LittleEndian.Uint16(response[len(response)-2:])

		if expectedCRC != actualCRC {
			return nil, fmt.Errorf("CRC mismatch: expected %04x, got %04x", expectedCRC, actualCRC)
		}
	}

	// Return data portion (exclude slave ID, function code, and CRC)
	if len(response) > 4 {
		return response[2 : len(response)-2], nil
	}

	return []byte{}, nil
}