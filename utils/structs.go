package utils

type Resp struct {
	Error any `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

func NewRespWithError(errStr ErrorMessage) *Resp {
	return &Resp{
		Error: errStr,
	}
}

func NewRespWithData(data any) *Resp {
	return &Resp{
		Data: data,
	}
}

type ErrorMessage string
