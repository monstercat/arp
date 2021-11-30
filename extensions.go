package arp

type ResponseParserAndValidator interface {
	ResponseValidator
	ResponseParser
}

type Extensions struct {
	ResponseType string
	Handler      ResponseParserAndValidator
}

var (
	// Configure what extensions are available to use here.
	AvailableExtensions = []Extensions{
		{
			ResponseType: "html",
			Handler:      &HtmlExt{},
		},
	}
)

func LoadExtensions(extList []string) (ResponseParserHandler, ResponseValidatorHandler) {
	extPool := AvailableExtensions
	if extList != nil && len(extList) > 0 {
		// todo: filter out based on the input list
	}

	respParser := ResponseParserHandler{}
	respParser.LoadDefaults()

	respValidator := ResponseValidatorHandler{}
	respValidator.LoadDefaults()

	for _, ext := range extPool {
		respParser.Register(ext.ResponseType, ext.Handler)
		respValidator.Register(ext.ResponseType, ext.Handler)
	}

	return respParser, respValidator
}
