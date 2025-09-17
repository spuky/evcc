package modbus

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Example usage of SolarmanV5 implementation for evcc
//
// This example demonstrates how to configure and use SolarmanV5
// protocol for communicating with solar inverters through
// Solarman data logging sticks.

func ExampleSolarmanV5Usage() {
	// Configuration example for evcc YAML:
	//
	// modbus:
	//   - name: solarman_inverter
	//     uri: 192.168.1.100:8899  # Solarman data logger IP and port
	//     id: 1                    # Modbus slave ID (inverter ID)
	//     solarmanv5: true         # Enable SolarmanV5 protocol
	//     loggerserial: 0x12345678 # Data logger serial number (required)

	// Create SolarmanV5 connection using evcc's modbus framework
	ctx := context.Background()

	settings := Settings{
		URI:          "192.168.1.100:8899",
		ID:           1, // Modbus slave ID
		SolarmanV5:   boolPtr(true),
		LoggerSerial: 0x12345678, // Replace with your actual logger serial
	}

	// This will create a SolarmanV5 connection
	conn, err := NewConnection(ctx, settings.URI, "", "", 0, settings.Protocol(), settings.ID)
	if err != nil {
		log.Fatalf("Failed to create connection: %v", err)
	}
	defer conn.Close()

	// Example: Read holding registers (solar inverter data)
	// This is equivalent to Modbus function code 03
	data, err := conn.ReadHoldingRegisters(100, 10) // Read 10 registers starting from address 100
	if err != nil {
		log.Printf("Failed to read holding registers: %v", err)
		return
	}

	fmt.Printf("Read %d bytes from inverter: % x\n", len(data), data)

	// Example: Read input registers (sensor data)
	// This is equivalent to Modbus function code 04
	inputData, err := conn.ReadInputRegisters(200, 5) // Read 5 registers starting from address 200
	if err != nil {
		log.Printf("Failed to read input registers: %v", err)
		return
	}

	fmt.Printf("Input registers: % x\n", inputData)
}

// Alternative direct usage without evcc's modbus framework
func ExampleDirectSolarmanV5Usage() {
	// Create a direct SolarmanV5 client
	client, err := NewSolarmanV5Client("192.168.1.100:8899", 0x12345678, 1)
	if err != nil {
		log.Fatalf("Failed to create SolarmanV5 client: %v", err)
	}
	defer client.Close()

	// Set timeout
	client.Timeout(5 * time.Second)

	// Read holding registers directly
	data, err := client.ReadHoldingRegisters(100, 10)
	if err != nil {
		log.Printf("Failed to read registers: %v", err)
		return
	}

	fmt.Printf("Direct read result: % x\n", data)
}

// Helper function for creating bool pointers
func boolPtr(b bool) *bool {
	return &b
}

// Configuration notes:
//
// 1. SolarmanV5 Protocol:
//    - Operates on TCP port 8899 (default)
//    - Encapsulates Modbus RTU frames in proprietary SolarmanV5 protocol
//    - Requires the data logger serial number for authentication
//
// 2. Finding Logger Serial:
//    - Usually found on a sticker on the data logging device
//    - May be available in the Solarman app or web interface
//    - Often a 32-bit hexadecimal number
//
// 3. Inverter Configuration:
//    - Different inverter brands may use different register mappings
//    - Consult your inverter's Modbus register documentation
//    - Common registers:
//      - Power: Often around registers 0x0084-0x0086
//      - Voltage: Often around registers 0x006D-0x0072
//      - Current: Often around registers 0x0076-0x007B
//      - Energy: Often around registers 0x0056-0x0063
//
// 4. Error Handling:
//    - The implementation automatically handles double CRC issues with some DEYE inverters
//    - Connection errors will trigger automatic reconnection
//    - Modbus errors are returned as standard Go errors
//
// 5. Integration with evcc:
//    - Use the standard evcc modbus meter/charger/vehicle templates
//    - Simply add solarmanv5: true and loggerserial to your configuration
//    - All existing register definitions and calculations work unchanged