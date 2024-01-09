package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
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

/*
	                              1  1  1  1  1  1
	0  1  2  3  4  5  6  7  8  9  0  1  2  3  4  5

+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
|                      ID                       |
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
|QR|   Opcode  |AA|TC|RD|RA|   Z    |   RCODE   |
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
|                    QDCOUNT                    |
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
|                    ANCOUNT                    |
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
|                    NSCOUNT                    |
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
|                    ARCOUNT                    |
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
*/
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

type DNSResourceRecord struct {
	Name     []byte
	Type     uint16
	Class    uint16
	TTL      uint32
	RDLength uint16
	RData    []byte
}

type DNSResponse struct {
	Header DNSHeader
	// Add other fields as needed for your response
	Question []DNSQuestion
	Answers  []DNSResourceRecord
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
		var RData [4]byte
		binary.BigEndian.PutUint32(RData[:], 134744072)

		receivedData := string(buf[:size])
		fmt.Printf("Received %d bytes from %s: %s\n", size, source, receivedData)
		var dnsHeader DNSHeader
		binary.Read(bytes.NewReader(buf[:12]), binary.BigEndian, &dnsHeader)

		// offset is from the start of the message, so need to use everything, but move 12 bytes
		reader := bytes.NewReader(buf[:size])
		reader.Seek(12, os.SEEK_SET)
		dnsQuestions := make([]DNSQuestion, 0)
		dnsAnswers := make([]DNSResourceRecord, 0)
		for {
			if reader.Len() == 0 {
				break
			}
			question, err := parseDNSQuestion(reader)
			if err != nil {
				log.Fatal("Error parsing DNS Question:", err)
			}
      fmt.Println("parsed question", question)
			dnsQuestions = append(dnsQuestions, *question)
			answer := DNSResourceRecord{
				Name:     question.Name,
				Type:     TypeA,
				Class:    1, // IN (Internet)
				TTL:      300,
				RDLength: 4,
				RData:    RData[:],
			}
			dnsAnswers = append(dnsAnswers, answer)
		}
		// Create an empty response
		response := DNSResponse{Header: dnsHeader,
			Question: dnsQuestions,
			Answers:  dnsAnswers,
		}
		response.Header.QDCount = uint16(len(dnsQuestions)) // we added one question
		response.Header.ANCount = uint16(len(dnsAnswers))
		response.Header.Flags |= (1 << 15) // set the QR (Query/Response) bit to indicate a response
		// RCODE is 0 (no error) if OPCODE is 0 (standard query) else 4 (not implemented)
		if (response.Header.Flags & 0x7800) != 0 {
			response.Header.Flags |= 4
		}
		respBytes, _ := packDNSResponse(response)

		_, err = udpConn.WriteToUDP(respBytes, source)
		if err != nil {
			fmt.Println("Failed to send response:", err)
		}
	}
}

func readName(reader *bytes.Reader) (string, error) {
	var labels []string
	for {
		// read the length byte
		length, err := reader.ReadByte()
		if err != nil {
			return "", err
		}

		if length == 0 {
			break // zero length indicate end of domain
		}

		// check if the label is compressed
		if (length >> 6) == 0x3 {
			// This is a pointer so read the next byte to form the 14-bit offset
			offsetByte, err := reader.ReadByte()
			if err != nil {
				return "", err
			}
			offset := (uint16(length&0x3F) << 8) | uint16(offsetByte)
			// save the current position
			curPos, _ := reader.Seek(0, os.SEEK_CUR)

			// move to the offset position
			_, err = reader.Seek(int64(offset), os.SEEK_SET)
			if err != nil {
				return "", err
			}

			// recursively read the name from new location
			label, err := readName(reader)
			if err != nil {
				return "", err
			}

			// Restore the original position
			_, err = reader.Seek(int64(curPos), os.SEEK_SET)
			if err != nil {
				return "", err
			}
			labels = append(labels, label)
			break
		}

		// Read the labels
		labelBytes := make([]byte, length)
		_, err = reader.Read(labelBytes)
		if err != nil {
			return "", err
		}
		labels = append(labels, string(labelBytes))
	}
	return strings.Join(labels, "."), nil
}

func parseDNSQuestion(reader *bytes.Reader) (*DNSQuestion, error) {
	name, err := readName(reader)
	if err != nil {
		return nil, err
	}
	var qType, qClass uint16
	binary.Read(reader, binary.BigEndian, &qType)
	binary.Read(reader, binary.BigEndian, &qClass)
	return &DNSQuestion{Name: labelSequence(name),
		Type:  qType,
		Class: qClass,
	}, nil
}

func packDNSResponse(response DNSResponse) ([]byte, error) {
	// Create a buffer to hold the binary representation
	size := 12
	for i := 0; i < int(response.Header.QDCount); i++ {
		size += len(response.Question[i].Name) + 4
	}

	// Calculate the length needed for the answer section
	for _, answer := range response.Answers {
		size += 2 + len(answer.Name) + 10 + len(answer.RData) // Name length + Type + Class + TTL + RDLength + RData length
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
	for _, question := range response.Question {
		qNameLength := len(question.Name)
		copy(buffer[offset:offset+qNameLength], question.Name)
		offset += qNameLength
		binary.BigEndian.PutUint16(buffer[offset:offset+2], question.Type)
		binary.BigEndian.PutUint16(buffer[offset+2:offset+4], question.Class)
		offset += 4
	}

	// Pack the DNS answer
	for _, answer := range response.Answers {
		nameLength := len(answer.Name)
		copy(buffer[offset:offset+nameLength], []byte(answer.Name))
		offset += nameLength
		binary.BigEndian.PutUint16(buffer[offset:offset+2], answer.Type)
		binary.BigEndian.PutUint16(buffer[offset+2:offset+4], answer.Class)
		binary.BigEndian.PutUint32(buffer[offset+4:offset+8], answer.TTL)
		binary.BigEndian.PutUint16(buffer[offset+8:offset+10], answer.RDLength)
		copy(buffer[offset+10:offset+14], answer.RData)
		offset += 14
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
