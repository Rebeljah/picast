package rtsp

type RTSPStatus string

const (
	Continue                      RTSPStatus = "100"
	OK                            RTSPStatus = "200"
	Created                       RTSPStatus = "201"
	LowOnStorageSpace             RTSPStatus = "250"
	MultipleChoices               RTSPStatus = "300"
	MovedPermanently              RTSPStatus = "301"
	MovedTemporarily              RTSPStatus = "302"
	SeeOther                      RTSPStatus = "303"
	UseProxy                      RTSPStatus = "305"
	BadRequest                    RTSPStatus = "400"
	Unauthorized                  RTSPStatus = "401"
	PaymentRequired               RTSPStatus = "402"
	Forbidden                     RTSPStatus = "403"
	NotFound                      RTSPStatus = "404"
	MethodNotAllowed              RTSPStatus = "405"
	NotAcceptable                 RTSPStatus = "406"
	ProxyAuthenticationRequired   RTSPStatus = "407"
	RequestTimeout                RTSPStatus = "408"
	Gone                          RTSPStatus = "410"
	LengthRequired                RTSPStatus = "411"
	PreconditionFailed            RTSPStatus = "412"
	RequestEntityTooLarge         RTSPStatus = "413"
	RequestURITooLong             RTSPStatus = "414"
	UnsupportedMediaType          RTSPStatus = "415"
	InvalidParameter              RTSPStatus = "451"
	IllegalConferenceIdentifier   RTSPStatus = "452"
	NotEnoughBandwidth            RTSPStatus = "453"
	SessionNotFound               RTSPStatus = "454"
	MethodNotValidInThisState     RTSPStatus = "455"
	HeaderFieldNotValid           RTSPStatus = "456"
	InvalidRange                  RTSPStatus = "457"
	ParameterIsReadOnly           RTSPStatus = "458"
	AggregateOperationNotAllowed  RTSPStatus = "459"
	OnlyAggregateOperationAllowed RTSPStatus = "460"
	UnsupportedTransport          RTSPStatus = "461"
	DestinationUnreachable        RTSPStatus = "462"
	InternalServerError           RTSPStatus = "500"
	NotImplemented                RTSPStatus = "501"
	BadGateway                    RTSPStatus = "502"
	ServiceUnavailable            RTSPStatus = "503"
	GatewayTimeout                RTSPStatus = "504"
	RTSPVersionNotSupported       RTSPStatus = "505"
	OptionNotSupported            RTSPStatus = "551"
)

var statusText = map[RTSPStatus]string{
	OK:                            "OK",
	Continue:                      "Continue",
	Created:                       "Created",
	LowOnStorageSpace:             "Low on Storage Space",
	MultipleChoices:               "Multiple Choices",
	MovedPermanently:              "Moved Permanently",
	MovedTemporarily:              "Moved Temporarily",
	SeeOther:                      "See Other",
	UseProxy:                      "Use Proxy",
	BadRequest:                    "Bad Request",
	Unauthorized:                  "Unauthorized",
	PaymentRequired:               "Payment Required",
	Forbidden:                     "Forbidden",
	NotFound:                      "Not Found",
	MethodNotAllowed:              "Method Not Allowed",
	NotAcceptable:                 "Not Acceptable",
	ProxyAuthenticationRequired:   "Proxy Authentication Required",
	RequestTimeout:                "Request Timeout",
	Gone:                          "Gone",
	LengthRequired:                "Length Required",
	PreconditionFailed:            "Precondition Failed",
	RequestEntityTooLarge:         "Request Entity Too Large",
	RequestURITooLong:             "Request-URI Too Long",
	UnsupportedMediaType:          "Unsupported Media Type",
	InvalidParameter:              "Invalid Parameter",
	IllegalConferenceIdentifier:   "Illegal Conference Identifier",
	NotEnoughBandwidth:            "Not Enough Bandwidth",
	SessionNotFound:               "Session Not Found",
	MethodNotValidInThisState:     "Method Not Valid in This State",
	HeaderFieldNotValid:           "Header Field Not Valid",
	InvalidRange:                  "Invalid Range",
	ParameterIsReadOnly:           "Parameter Is Read-Only",
	AggregateOperationNotAllowed:  "Aggregate Operation Not Allowed",
	OnlyAggregateOperationAllowed: "Only Aggregate Operation Allowed",
	UnsupportedTransport:          "Unsupported Transport",
	DestinationUnreachable:        "Destination Unreachable",
	InternalServerError:           "Internal Server Error",
	NotImplemented:                "Not Implemented",
	BadGateway:                    "Bad Gateway",
	ServiceUnavailable:            "Service Unavailable",
	GatewayTimeout:                "Gateway Timeout",
	RTSPVersionNotSupported:       "RTSP Version Not Supported",
	OptionNotSupported:            "Option Not Supported",
}

func (s RTSPStatus) String() string {
	return statusText[s]
}
