package arp

type ResponseValidator interface {
	Validate(test *TestCase, result *TestResult) (bool, []*FieldMatcherResult, error)
}

type ResponseValidatorHandler map[string]ResponseValidator

func (rvh *ResponseValidatorHandler) Register(responseType string, handler ResponseValidator) {
	(*rvh)[responseType] = handler
}

func (rvh *ResponseValidatorHandler) LoadDefaults() {
	(*rvh) = make(map[string]ResponseValidator)
	rvh.Register("rest", &JSONParser{})
}

func (rvh *ResponseValidatorHandler) Handle(test *TestCase, result *TestResult) (bool, []*FieldMatcherResult, error) {

	// if a custom validator is given, use it
	if validator, exists := (*rvh)[test.Config.Response.Type]; exists {
		return validator.Validate(test, result)
	}

	// otherwise fall back to the built-in ones
	if test.Config.Websocket {
		return test.ResponseMatcher.Match(result.Response)
	} else if !test.IsRPC {
		return (*rvh)["rest"].Validate(test, result)
	} else {
		return test.ResponseMatcher.Match(result.Response)
	}
}
