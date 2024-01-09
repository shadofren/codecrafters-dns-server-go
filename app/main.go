package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

type DNSHeader struct {
	ID      uint16
	Flags   uint16 // embedded struct for multiple flags
	QDCount uint16
	ANCount uint16
	NSCount uint16
	ARCount uint16
}

type DNSResponse struct {
	Header DNSHeader
	// Add other fields as needed for your response
}

func main() {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:2053")
	if err != nil {
		fmt.Println("Failed to resolve UDP address:", err)
		return
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println("Failed to bind to address:", err)
		return
	}
	defer udpConn.Close()

	buf := make([]byte, 512)

	for {
		size, source, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			fmt.Println("Error receiving data:", err)
			break
		}

		receivedData := string(buf[:size])
		fmt.Printf("Received %d bytes from %s: %s\n", size, source, receivedData)
		var dnsHeader DNSHeader
		binary.Read(bytes.NewReader(buf[:12]), binary.BigEndian, &dnsHeader)

		// Create an empty response
		response := DNSResponse{dnsHeader}
    response.Header.Flags |= (1 << 15) // set the QR (Query/Response) bit to indicate a response
    respBytes, _ := packDNSResponse(response)

		_, err = udpConn.WriteToUDP(respBytes, source)
		if err != nil {
			fmt.Println("Failed to send response:", err)
		}
	}
}

func packDNSResponse(response DNSResponse) ([]byte, error) {
	// Create a buffer to hold the binary representation
	buffer := make([]byte, 12) // Adjust the size as needed

	// Pack the DNS header
	binary.BigEndian.PutUint16(buffer[0:2], response.Header.ID)
	binary.BigEndian.PutUint16(buffer[2:4], response.Header.Flags)
	binary.BigEndian.PutUint16(buffer[4:6], response.Header.QDCount)
	binary.BigEndian.PutUint16(buffer[6:8], response.Header.ANCount)
	binary.BigEndian.PutUint16(buffer[8:10], response.Header.NSCount)
	binary.BigEndian.PutUint16(buffer[10:12], response.Header.ARCount)

	// Add other code to pack additional fields in the response

	return buffer, nil
}
