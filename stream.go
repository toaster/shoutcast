package shoutcast

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"
)

// MetadataCallbackFunc is the type of the function called when the stream metadata changes
type MetadataCallbackFunc func(m *Metadata)

// Stream represents an open shoutcast stream.
type Stream struct {
	// The name of the server
	Name string

	// What category the server falls under
	Genre string

	// The description of the stream
	Description string

	// Homepage of the server
	URL string

	// Bitrate of the server
	Bitrate int

	// Optional function to be executed when stream metadata changes
	MetadataCallbackFunc MetadataCallbackFunc

	// Amount of bytes to read before expecting a metadata block
	metaint int

	// Stream metadata
	metadata *Metadata

	// The number of bytes read since last metadata block
	pos int

	// The underlying data stream
	rc io.ReadCloser
}

// Open establishes a connection to a remote server.
func Open(url string) (*Stream, error) {
	log.Print("[INFO] Opening ", url)

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Add("accept", "*/*")
	req.Header.Add("user-agent", "iTunes/12.9.2 (Macintosh; OS X 10.14.3) AppleWebKit/606.4.5")
	req.Header.Add("icy-metadata", "1")

	// Timeout for establishing the connection.
	// We don't want for the stream to timeout while we're reading it, but
	// we do want a timeout for establishing the connection to the server.
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Dial: dialer.Dial}
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	for k, v := range resp.Header {
		log.Print("[DEBUG] HTTP header ", k, ": ", v[0])
	}

	bitrate, err := strconv.Atoi(resp.Header.Get("icy-br"))
	if err != nil {
		return nil, fmt.Errorf("cannot parse bitrate: %v", err)
	}

	metaint, err := strconv.Atoi(resp.Header.Get("icy-metaint"))
	if err != nil {
		return nil, fmt.Errorf("cannot parse metaint: %v", err)
	}

	s := &Stream{
		Name:        resp.Header.Get("icy-name"),
		Genre:       resp.Header.Get("icy-genre"),
		Description: resp.Header.Get("icy-description"),
		URL:         resp.Header.Get("icy-url"),
		Bitrate:     bitrate,
		metaint:     metaint,
		metadata:    nil,
		pos:         0,
		rc:          resp.Body,
	}

	return s, nil
}

// Read implements the standard Read interface
func (s *Stream) Read(p []byte) (n int, err error) {
	n, err = s.rc.Read(p)

	if s.pos+n <= s.metaint {
		s.pos = s.pos + n
		return n, err
	}

	// extract stream metadata
	metadataStart := s.metaint - s.pos
	metadataLength := int(p[metadataStart : metadataStart+1][0]) * 16
	if metadataLength > 0 {
		m := NewMetadata(p[metadataStart+1 : metadataStart+1+metadataLength])
		if !m.Equals(s.metadata) {
			s.metadata = m
			if s.MetadataCallbackFunc != nil {
				s.MetadataCallbackFunc(s.metadata)
			}
		}
	}

	// roll over position + metadata block
	s.pos = ((s.pos + n) - s.metaint) - metadataLength - 1

	// shift buffer data to account for metadata block
	copy(p[metadataStart:], p[metadataStart+1+metadataLength:])
	n = n - 1 - metadataLength

	return n, err
}

// Close closes the stream
func (s *Stream) Close() error {
	log.Print("[INFO] Closing ", s.URL)
	return s.rc.Close()
}
