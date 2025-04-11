package rtsp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	HeaderNameTransport         string = "Transport"
	HeaderNameAccept            string = "Accept"
	HeaderNameAcceptEncoding    string = "Accept-Encoding"
	HeaderNameAcceptLanguage    string = "Accept-Language"
	HeaderNameAllow             string = "Allow"
	HeaderNameAuthorization     string = "Authorization"
	HeaderNameBandwidth         string = "Bandwidth"
	HeaderNameBlocksize         string = "Blocksize"
	HeaderNameCacheControl      string = "Cache-Control"
	HeaderNameConference        string = "Conference"
	HeaderNameConnection        string = "Connection"
	HeaderNameContentBase       string = "Content-Base"
	HeaderNameContentEncoding   string = "Content-Encoding"
	HeaderNameContentLanguage   string = "Content-Language"
	HeaderNameContentLength     string = "Content-Length"
	HeaderNameContentLocation   string = "Content-Location"
	HeaderNameContentType       string = "Content-Type"
	HeaderNameCSeq              string = "CSeq"
	HeaderNameDate              string = "Date"
	HeaderNameExpires           string = "Expires"
	HeaderNameFrom              string = "From"
	HeaderNameIfModifiedSince   string = "If-Modified-Since"
	HeaderNameLastModified      string = "Last-Modified"
	HeaderNameProxyAuthenticate string = "Proxy-Authenticate"
	HeaderNameProxyRequire      string = "Proxy-Require"
	HeaderNamePublic            string = "Public"
	HeaderNameRange             string = "Range"
	HeaderNameReferer           string = "Referer"
	HeaderNameRequire           string = "Require"
	HeaderNameRetryAfter        string = "Retry-After"
	HeaderNameRTPInfo           string = "RTP-Info"
	HeaderNameScale             string = "Scale"
	HeaderNameSession           string = "Session"
	HeaderNameServer            string = "Server"
	HeaderNameSpeed             string = "Speed"
	HeaderNameUnsupported       string = "Unsupported"
	HeaderNameUserAgent         string = "User-Agent"
	HeaderNameVia               string = "Via"
	HeaderNameWWWAuthenticate   string = "WWW-Authenticate"
)

type HeaderLine interface {
	// implement by returning a complete header line with `\r\n`
	Marshal() ([]byte, error)

	Name() string

	// returns the value
	Value() (string, error)

	// returns the value, if the value can't be marshalled, returns an empty string
	ValueNoError() string
}

type GenericHeaderLine struct {
	name     string
	rawValue string
}

func NewGenericHeaderLine(name string, value string) GenericHeaderLine {
	return GenericHeaderLine{
		name:     name,
		rawValue: value,
	}
}

func (h GenericHeaderLine) Value() (string, error) {
	return h.rawValue, nil
}

func (h GenericHeaderLine) ValueNoError() string {
	v, err := h.Value()

	if err != nil {
		return ""
	}

	return v
}

func (h GenericHeaderLine) Name() string {
	return h.name
}

func (h GenericHeaderLine) Marshal() ([]byte, error) {
	return fmt.Appendf(nil, "%s: %s\r\n", h.Name(), h.ValueNoError()), nil
}

type Headers map[string]HeaderLine

func NewHeadersFromString(s string) (Headers, error) {
	headers := make(Headers)

	s = strings.Trim(s, "\r\n")

	for line := range strings.SplitSeq(s, "\r\n") {
		hl, err := ParseHeaderLine(line)

		if err != nil {
			return nil, err
		}

		headers[hl.Name()] = hl
	}

	return headers, nil
}

func (h Headers) Marshal() ([]byte, error) {
	head := make([]byte, 0)

	// write each header field
	for _, headerLine := range h {
		line, err := headerLine.Marshal()
		if err != nil {
			return nil, err
		}

		head = append(head, line...)
	}

	return head, nil
}

func (h Headers) PutLine(hl HeaderLine) {
	h[hl.Name()] = hl
}

func (h Headers) PutGenericLine(name string, value string) {
	h[name] = GenericHeaderLine{name: name, rawValue: value}
}

func (h Headers) GetLine(name string) (HeaderLine, bool) {
	hl, ok := h[name]
	return hl, ok
}

// returns an empty HeaderLine if the field name doesn't exist in the headers
func (h Headers) GetLineNoFail(name string) HeaderLine {
	if hl, ok := h.GetLine(name); ok {
		return hl
	}

	return GenericHeaderLine{}
}

func (h Headers) Delete(name string) bool {
	_, ok := h[name]
	delete(h, name)
	return ok
}

type TransportInfo struct {
	Protocol        string // RTP
	Profile         string // AVP
	Mode            string // "unicast"
	ClientPortStart int    // start of the [...) port range
	ClientPortEnd   int    // end of the [...) port range
}

type TransportHeaderLine struct {
	GenericHeaderLine
	Transports []TransportInfo
}

func NewTransportHeaderLine(transports []TransportInfo) TransportHeaderLine {
	return TransportHeaderLine{
		GenericHeaderLine: NewGenericHeaderLine("Transport", ""),
		Transports:        make([]TransportInfo, 0),
	}
}

func ParseTransportHeaderLine(ln string) TransportHeaderLine {
	// Remove "Transport: " prefix
	valueStr := strings.TrimPrefix(ln, "Transport: ")
	valueStr = strings.Trim(valueStr, " \r\n")

	// Split multiple transport specs
	transportSpecs := strings.Split(valueStr, ",")
	transports := make([]TransportInfo, 0, len(transportSpecs))

	for _, spec := range transportSpecs {
		spec = strings.TrimSpace(spec)
		parts := strings.Split(spec, ";")

		// Parse protocol/profile (first part)
		protoParts := strings.Split(parts[0], "/")
		protocol := protoParts[0]
		profile := protoParts[1]

		// Parse mode (second part)
		mode := parts[1]

		// Parse client ports (third part)
		portRange := strings.TrimPrefix(parts[2], "client_port=")
		ports := strings.Split(portRange, "-")
		portStart, _ := strconv.Atoi(ports[0])
		portEnd, _ := strconv.Atoi(ports[1])

		transports = append(transports, TransportInfo{
			Protocol:        protocol,
			Profile:         profile,
			Mode:            mode,
			ClientPortStart: portStart,
			ClientPortEnd:   portEnd,
		})
	}

	return TransportHeaderLine{
		GenericHeaderLine: NewGenericHeaderLine(HeaderNameTransport, valueStr),
		Transports:        transports,
	}
}

// TODO this function doesnt check for errors in the formatting
func (h TransportHeaderLine) Marshal() ([]byte, error) {
	line := fmt.Append(nil, string(h.name)+": ")

	for i, trspt := range h.Transports {
		line = fmt.Appendf(line, "%s/%s;%s;client_port=%d-%d",
			trspt.Protocol,
			trspt.Profile,
			trspt.Mode,
			trspt.ClientPortStart,
			trspt.ClientPortEnd,
		)

		if i+1 < len(h.Transports) {
			line = append(line, ',')
		}
	}

	return append(line, "\r\n"...), nil
}

func ParseHeaderLine(line string) (HeaderLine, error) {
	line = strings.Trim(line, "\r\n ")

	// validate "k: v" format
	if !strings.Contains(line, ": ") {
		return nil, errors.New("header line not in 'k: v' format")
	}

	// split name and val
	sp := strings.SplitN(line, ": ", 2)
	name := sp[0]
	val := sp[1]

	// name or val cannot be empty
	if len(name) == 0 || len(val) == 0 {
		return nil, errors.New("empty name or value in header line")
	}

	// handle structured header fields first, the rest can be GenericHeaderLine by default.
	switch name {
	case HeaderNameTransport:
		return ParseTransportHeaderLine(line), nil
	default:
		return GenericHeaderLine{name: name, rawValue: val}, nil
	}
}
