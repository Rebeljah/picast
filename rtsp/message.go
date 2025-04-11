// RFC2326

package rtsp

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const RTSP_VERSION_STRING string = "RTSP/1.0"

var ErrBadRequest = errors.New("bad request")
var ErrInvalidFormat = errors.New("cannot parse message")

type Message struct {
	Headers Headers
	Body    []byte
}

func NewMessageFromString(s string) (Message, error) {
	// if not even headers are present, return a nil Message
	if len(s) == 0 {
		return Message{}, nil
	}

	bodyDeliniatorPosition := strings.Index(s, "\r\n\r\n")

	if bodyDeliniatorPosition == -1 {
		return Message{}, ErrInvalidFormat
	}

	bodyStartPosition := bodyDeliniatorPosition + 4

	var headers Headers
	var body []byte = make([]byte, 0)

	var err error

	headers, err = NewHeadersFromString(s[0:bodyDeliniatorPosition])
	if err != nil {
		return Message{}, err
	}

	if bodyStartPosition < len(s) { // has body
		body = []byte(s[bodyStartPosition:])
	}

	return Message{
		Headers: headers,
		Body:    body,
	}, nil
}

func (m Message) Marshal() ([]byte, error) {
	head, err := m.Headers.Marshal()
	if err != nil {
		return nil, err
	}

	return fmt.Appendf(nil, "%s\r\n%s", head, m.Body), nil
}

type RequestLine struct {
	Method  RTSPMethod
	URL     *url.URL
	Version string
}

type Request struct {
	RequestLine
	Message
}

func (r Request) Marshal() ([]byte, error) {
	buf := make([]byte, 0)

	fmt.Appendf(buf, "%s %s %s\r\n", string(r.Method), r.URL.String(), r.Version)

	msg, err := r.Message.Marshal()

	if err != nil {
		return nil, err
	}

	return append(buf, msg...), nil
}

func NewRequest(method RTSPMethod, url *url.URL) Request {
	return Request{
		RequestLine: RequestLine{
			Method:  method,
			URL:     url,
			Version: RTSP_VERSION_STRING,
		},
	}
}

func newRequestFromString(s string) (Request, error) {
	messageDelineatorPosition := strings.Index(s, "\r\n")
	headerStart := messageDelineatorPosition + 2
	requestLineParts := strings.SplitN(s[0:messageDelineatorPosition], " ", 3)

	// note that requests must never be malformed as long as
	// this project remains for secured-LAN-only use.
	// malformed requests are treated as serious security concerns, so panic.

	method := RTSPMethod(requestLineParts[0])

	if !IsValidRTSPMethod(requestLineParts[0]) {
		return Request{}, ErrBadRequest
	}

	url, err := url.Parse(requestLineParts[1])

	if err != nil {
		return Request{}, ErrBadRequest
	}

	version := requestLineParts[2]

	if version != RTSP_VERSION_STRING {
		return Request{}, ErrBadRequest
	}

	request := NewRequest(method, url)

	if headerStart < len(s) {
		request.Message, err = NewMessageFromString(s[headerStart:])
		if err != nil {
			return Request{}, err
		}
	}

	return request, nil
}

type requestContext struct {
	raddr    net.Addr
	request  *Request
	response *Response
	session  *Session
}

func newRequestContext(raddr net.Addr, req *Request, resp *Response, session *Session) *requestContext {
	return &requestContext{
		raddr:    raddr,
		request:  req,
		response: resp,
		session:  session,
	}
}

type ResponseLine struct {
	Version    string
	StatusCode RTSPStatus
	StatusText string
}

type Response struct {
	ResponseLine
	Message
}

func newResponse(statusCode RTSPStatus) *Response {
	return &Response{
		ResponseLine: ResponseLine{
			Version:    RTSP_VERSION_STRING,
			StatusCode: statusCode,
			StatusText: statusCode.String(),
		},
	}
}

func (r *Response) marshal() ([]byte, error) {
	msgbuf, err := r.Message.Marshal()
	if err != nil {
		return nil, err
	}

	return fmt.Appendf(nil, "%s %s %s\r\n%s", r.Version, r.StatusCode, r.StatusText, msgbuf), nil
}

func (r *Response) writeHeader(c RTSPStatus) {
	r.StatusCode = c
	r.StatusText = r.StatusCode.String()
}

func (r *Response) writeBody(b []byte) {
	r.Body = make([]byte, len(b))
	copy(r.Body, b)
}

// calls WriteHeader with the given status and sets the message body to the error text
func (r *Response) writeError(c RTSPStatus, err error) {
	r.writeHeader(c)
	r.writeBody([]byte(err.Error()))
}
