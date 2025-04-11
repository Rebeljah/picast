package rtsp

// RFC2326-10
type RTSPMethod string

const (
	OPTIONS       RTSPMethod = "OPTIONS"
	DESCRIBE      RTSPMethod = "DESCRIBE"
	ANNOUNCE      RTSPMethod = "ANNOUNCE"
	SETUP         RTSPMethod = "SETUP"
	PLAY          RTSPMethod = "PLAY"
	PAUSE         RTSPMethod = "PAUSE"
	TEARDOWN      RTSPMethod = "TEARDOWN"
	GET_PARAMETER RTSPMethod = "GET_PARAMETER"
	SET_PARAMETER RTSPMethod = "SET_PARAMETER"
	REDIRECT      RTSPMethod = "REDIRECT"
	RECORD        RTSPMethod = "RECORD"
)

var validRTSPMethods = map[string]RTSPMethod{
	"OPTIONS":       OPTIONS,
	"DESCRIBE":      DESCRIBE,
	"ANNOUNCE":      ANNOUNCE,
	"SETUP":         SETUP,
	"PLAY":          PLAY,
	"PAUSE":         PAUSE,
	"TEARDOWN":      TEARDOWN,
	"GET_PARAMETER": GET_PARAMETER,
	"SET_PARAMETER": SET_PARAMETER,
	"REDIRECT":      REDIRECT,
	"RECORD":        RECORD,
}

func IsValidRTSPMethod(method string) bool {
	_, exists := validRTSPMethods[method]
	return exists
}
