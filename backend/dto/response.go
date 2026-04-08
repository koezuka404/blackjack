package dto

type SuccessResponse[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

func OK[T any](data T) SuccessResponse[T] {
	return SuccessResponse[T]{Success: true, Data: data}
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type FailResponse struct {
	Success bool     `json:"success"`
	Error   APIError `json:"error"`
}

func Fail(code, message string) FailResponse {
	return FailResponse{
		Success: false,
		Error: APIError{
			Code:    code,
			Message: message,
		},
	}
}
