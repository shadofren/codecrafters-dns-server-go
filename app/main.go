package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

const (
	// Common DNS question types
	TypeA     = 1   // IPv4 address
	TypeNS    = 2   // Name server
	TypeCNAME = 5   // Canonical name
	TypeMX    = 15  // Mail exchange
	TypeAAAA  = 28  // IPv6 address
	TypeSRV   = 33  // Service location
	TypeTXT   = 16  // Text strings
	TypePTR   = 12  // Pointer record
	TypeSOA   = 6   // Start of authority
	TypeANY   = 255 // Wildcard match any type
)

type DNSHeader struct {
	ID      uint16
	Flags   uint16 // embedded struct for multiple flags
	QDCount uint16
	ANCount uint16
	NSCount uint16
	ARCount uint16
}

type DNSQuestion struct {
	Name  []byte
	Type  uint16
	Class uint16
}

type DNSResponse struct {
	Header DNSHeader
	// Add other fields as needed for your response
	Questions []DNSQuestion
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
		response := DNSResponse{Header: dnsHeader, Questions: make([]DNSQuestion, 0)}
		response.Header.ID = 1234
		response.Questions = append(response.Questions, DNSQuestion{
			Name:  labelSequence("codecrafters.io"),
			Type:  TypeA,
			Class: 1, // IN (Internet)
		})
		response.Header.QDCount = 1        // we added one question
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
  size := 12
  for i := 0; i < int(response.Header.QDCount); i++ {
    size += len(response.Questions[i].Name) + 4
  }
	buffer := make([]byte, size) // Adjust the size as needed

	// Pack the DNS header
	binary.BigEndian.PutUint16(buffer[0:2], response.Header.ID)
	binary.BigEndian.PutUint16(buffer[2:4], response.Header.Flags)
	binary.BigEndian.PutUint16(buffer[4:6], response.Header.QDCount)
	binary.BigEndian.PutUint16(buffer[6:8], response.Header.ANCount)
	binary.BigEndian.PutUint16(buffer[8:10], response.Header.NSCount)
	binary.BigEndian.PutUint16(buffer[10:12], response.Header.ARCount)

	// Pack the DNS Questions
	offset := 12
	for i := 0; i < int(response.Header.QDCount); i++ {
		qNameLength := len(response.Questions[i].Name)
		copy(buffer[offset:offset+qNameLength], response.Questions[i].Name)
		offset += qNameLength
		binary.BigEndian.PutUint16(buffer[offset:offset+2], response.Questions[i].Type)
		binary.BigEndian.PutUint16(buffer[offset+2:offset+4], response.Questions[i].Class)
		offset += 4
	}

	return buffer, nil
}

func labelSequence(domain string) []byte {
	labels := strings.Split(domain, ".")
	var sequence []byte
	for _, label := range labels {
		sequence = append(sequence, byte(len(label)))
		sequence = append(sequence, label...)
	}
	sequence = append(sequence, '\x00')
	return sequence
}

func intToByte(n int) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(n))
	return b
}
