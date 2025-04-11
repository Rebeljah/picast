package rtsp

// note this package can likely most be mostly replaced with net.http with some hacks, this
// is just for fun.

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/rebeljah/picast/media"
)

type handler interface {
	serveRTSP(*requestContext)
	withMiddleware(handler) handler
}

type serveMux map[RTSPMethod]handler

func newDefaultMux() serveMux {
	return make(serveMux)
}

func (m serveMux) handle(method RTSPMethod, handler handler) {
	m[method] = handler
}

func (m serveMux) serveRTSP(ctx *requestContext) {
	handler, ok := m[ctx.request.Method]

	if !ok {
		ctx.response.writeHeader(MethodNotAllowed)
		return
	}

	handler.serveRTSP(ctx)
}

func (m serveMux) withMiddleware(mdl handler) handler {
	return newMiddleWare(mdl, m)
}

// HandlerFunc type is an adapter to allow the use of
// ordinary functions as RTSP handlers. If f is a function
// with the appropriate signature, HandlerFunc(f) is a
// Handler that calls f.
type HandlerFunc func(*requestContext)

// serveRTSP calls f(ctx) to implement Handler.
func (f HandlerFunc) serveRTSP(ctx *requestContext) {
	f(ctx)
}

func (f HandlerFunc) withMiddleware(mdl handler) handler {
	return newMiddleWare(mdl, f)
}

type Middleware struct {
	handler     handler
	nextHandler handler
}

func newMiddleWare(handler handler, nextHandler handler) Middleware {
	return Middleware{handler: handler, nextHandler: nextHandler}
}

func (m Middleware) serveRTSP(ctx *requestContext) {
	m.handler.serveRTSP(ctx)

	if ctx.response.StatusCode == OK {
		m.nextHandler.serveRTSP(ctx)
	}
}

func (m Middleware) withMiddleware(mdl handler) handler {
	return newMiddleWare(mdl, m)
}

func handleMirrorCSeqHeader(ctx *requestContext) {
	cseq, ok := ctx.request.Headers.GetLine(HeaderNameCSeq)
	if !ok {
		ctx.response.writeHeader(BadRequest)
		return
	}

	_, err := strconv.Atoi(string(cseq.ValueNoError()))

	if err != nil {
		ctx.response.writeHeader(BadRequest)
		return
	}

	ctx.response.Headers.PutGenericLine("CSeq", cseq.ValueNoError())
}

func handleSettingFinalHeaders(ctx *requestContext) {
	// content-length
	if n := len(ctx.response.Body); n == 0 {
		ctx.response.Headers.Delete(HeaderNameContentLength)
	} else {
		ctx.response.Headers.PutGenericLine(
			HeaderNameContentLength, strconv.Itoa(n),
		)
	}

	ctx.response.Headers.PutGenericLine(
		HeaderNameConnection, "close",
	)
}

type RTSPServer struct {
	sessions      sessionManager
	handler       handler
	rtpServer     RTPServer
	mediaManifest media.Manifest
	listener      net.Listener
	interruptOnce sync.Once
}

func NewRTSPServer(rtpServer RTPServer, manifest media.Manifest) *RTSPServer {
	s := &RTSPServer{
		sessions:      newSessionManager(),
		mediaManifest: manifest,
		rtpServer:     rtpServer,
	}

	mux := newDefaultMux()
	mux.handle(SETUP, HandlerFunc(s.handleSetup))
	mux.handle(TEARDOWN, HandlerFunc(s.handleTeardown))
	mux.handle(PLAY, HandlerFunc(s.handlePlay))
	mux.handle(PAUSE, HandlerFunc(s.handlePause))
	mux.handle(OPTIONS, HandlerFunc(s.handleOptions))

	s.handler = HandlerFunc(handleSettingFinalHeaders)
	s.handler = mux
	s.handler = s.handler.withMiddleware(HandlerFunc(s.handleSettingContextSession))
	s.handler = s.handler.withMiddleware(HandlerFunc(handleMirrorCSeqHeader))

	return s
}

func (s *RTSPServer) ListenAndServe(addr string) error {
	log.Println("starting RTSP server on " + addr)

	ls, err := net.Listen("tcp", addr)

	if err != nil {
		return err
	}

	s.listener = ls
	defer s.listener.Close()

	for {
		conn, err := s.listener.Accept()

		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}

			log.Printf("RTSP listener accept error: %v", err)

			continue
		}

		go s.serveConnection(conn)
	}
}

func (s *RTSPServer) Interrupt(err error) {
	s.interruptOnce.Do(func() {
		log.Printf("Interrupting RTSP server: %v\n", err)

		s.listener.Close()

		log.Println("RTSP server shutdown complete")
	})
}

func (s *RTSPServer) handleSetup(ctx *requestContext) {
	path := strings.Trim(ctx.request.URL.Path, "/ ")
	segments := strings.Split(path, "/")

	// media/{uid}
	if len(segments) != 2 {
		ctx.response.writeHeader(NotFound)
		return
	}

	if segments[0] != "media" {
		ctx.response.writeHeader(MethodNotAllowed)
		return
	}

	mediaUID := media.UID(segments[1])

	metadata, ok := s.mediaManifest.Get(mediaUID)

	if !ok {
		ctx.response.writeHeader(NotFound)
		return
	}

	ctx.session.Stream = NewStreamState()

	if ok && ctx.session.Stream.StateNow != Init {
		ctx.response.writeHeader(MethodNotValidInThisState)
		return
	}

	line, ok := ctx.request.Headers.GetLine(HeaderNameTransport)

	if !ok {
		ctx.response.writeHeader(BadRequest)
		return
	}

	var transportHeader TransportHeaderLine
	if transportHeader, ok = line.(TransportHeaderLine); !ok {
		ctx.response.writeHeader(InternalServerError)
		return
	}

	if len(transportHeader.Transports) == 0 {
		ctx.response.writeHeader(BadRequest)
		return
	}

	args := newSetupArguments(
		ctx.session.Stream.StreamUID,
		ctx.raddr,
		metadata.Structure,
		transportHeader.Transports,
	)

	transport, err := s.rtpServer.SetupStream(args)

	if err != nil {
		ctx.response.writeHeader(InternalServerError)
		return
	}

	ctx.response.Headers.PutGenericLine(
		HeaderNameSession, string(ctx.session.UID),
	)

	ctx.response.Headers.PutLine(
		NewTransportHeaderLine([]TransportInfo{transport}),
	)

	ctx.session.Stream.OnSetup()
}

func (s *RTSPServer) handleTeardown(ctx *requestContext) {
	// validate media/{id}

	/////////////////// TODO section is repeated in HandleSetup (extract?)
	path := strings.Trim(ctx.request.URL.Path, "/ ")
	segments := strings.Split(path, "/")

	if n := len(segments); n != 2 {
		ctx.response.writeHeader(NotFound)
		return
	}

	if segments[0] != "media" {
		ctx.response.writeHeader(MethodNotAllowed)
		return
	}
	///////////////////////

	st := ctx.session.Stream

	if st == nil {
		ctx.response.writeHeader(NotFound)
		return
	}

	// make sure stream can actually be torn down in current state
	if st.StateNow.After(TEARDOWN) == ErrorState {
		ctx.response.writeHeader(MethodNotValidInThisState)
		return
	}

	s.rtpServer.TeardownStream(st.StreamUID)
	st.OnTeardown()

	s.sessions.delete(ctx.session.UID)
}

func (*RTSPServer) handlePlay(ctx *requestContext) {}

func (*RTSPServer) handlePause(ctx *requestContext) {}

func (*RTSPServer) handleOptions(ctx *requestContext) {}

func (s *RTSPServer) handleSettingContextSession(ctx *requestContext) {
	sessionHeader, ok := ctx.request.Headers.GetLine(HeaderNameSession)

	// context not required for SETUP, OPTIONS
	if !ok && ctx.request.Method != SETUP && ctx.request.Method != OPTIONS {
		ctx.response.writeHeader(SessionNotFound)
		return
	}

	// SETUP not valid for an active streaming session
	if ok && ctx.request.Method == SETUP {
		ctx.response.writeHeader(MethodNotValidInThisState)
		return
	}

	sessionUID := SessionUID(sessionHeader.ValueNoError())
	ctx.session, ok = s.sessions.get(sessionUID)

	if !ok {
		ctx.response.writeHeader(SessionNotFound)
		return
	}
}

func (s *RTSPServer) readRequest(conn net.Conn) (Request, error) {
	var raw strings.Builder
	reader := bufio.NewReader(conn)

	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			return Request{}, err
		}

		raw.WriteString(line)

		if line == "\r\n" {
			break
		}
	}

	request, err := newRequestFromString(raw.String())

	if err != nil {
		return Request{}, err
	}

	if contentLenHeaderLine, ok := request.Headers.GetLine(HeaderNameContentLength); ok {
		contentLength, err := strconv.Atoi(string(contentLenHeaderLine.ValueNoError()))

		if err != nil {
			return request, err
		}

		request.Body = make([]byte, contentLength)
		_, err = io.ReadFull(conn, request.Body)

		if err != nil {
			return request, err
		}
	}

	return request, nil
}

func (s *RTSPServer) serveConnection(conn net.Conn) {
	log.Printf("serving RTSP to: %v", conn.RemoteAddr())

	var resp []byte
	var rctx *requestContext
	raddr := conn.RemoteAddr()

	defer conn.Close()
	defer func() {
		if resp == nil { // Only write if resp was set
			return
		}

		log.Printf("writing RTSP response to: %v", raddr)

		_, err := conn.Write(resp)
		if err != nil {
			log.Printf("RTSP write error to %v: %v\n", raddr, err)
			return
		}

		log.Printf("wrote RTSP response to: %v (%v %v)", raddr, rctx.response.StatusCode, rctx.response.StatusText)
	}()

	log.Printf("reading RTSP request from: %v", raddr)

	req, err := s.readRequest(conn)
	if err != nil {
		log.Printf("RTSP read error from %v: %v\n", raddr, err)
		return
	}

	rctx = newRequestContext(conn.RemoteAddr(), &req, newResponse(OK), nil)

	log.Printf("handling RTSP request from: %v (%v %v)", raddr, req.Method, req.URL)

	s.handler.serveRTSP(rctx)

	resp, err = rctx.response.marshal()
	if err != nil {
		resp, _ = newResponse(InternalServerError).marshal()
		log.Printf("error while marshalling RTSP response to: %v", raddr)
		return
	}
}
