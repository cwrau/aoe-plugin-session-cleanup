// Package rpc speaks JSON-RPC 2.0 over newline-delimited JSON on stdio with the
// aoe host: the message shapes and response builders here, and the outbound
// host calls and inbound demux in host.go.
//
// A MethodNotFoundError becomes -32601; anything else is the internal-error
// code -32603.
package rpc

import "encoding/json"

// JSON-RPC error codes the worker answers with.
const (
	CodeMethodNotFound = -32601
	CodeInternal       = -32603
)

// ErrorBody is the `error` member of a JSON-RPC response.
type ErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Response is a JSON-RPC response to a host request.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *ErrorBody      `json:"error,omitempty"`
}

// MethodNotFoundError is returned by a dispatcher for a method it does not know.
type MethodNotFoundError struct {
	Method string
}

func (e *MethodNotFoundError) Error() string {
	return "unknown method " + e.Method
}

// ResultResponse builds the success response for request id.
func ResultResponse(id json.RawMessage, result any) Response {
	return Response{JSONRPC: "2.0", ID: id, Result: result}
}

// ErrorResponse builds the error response for request id.
func ErrorResponse(id json.RawMessage, err error) Response {
	if mnf, ok := err.(*MethodNotFoundError); ok {
		return Response{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &ErrorBody{Code: CodeMethodNotFound, Message: "unknown method " + mnf.Method},
		}
	}
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &ErrorBody{Code: CodeInternal, Message: err.Error()},
	}
}
