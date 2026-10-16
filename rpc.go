// JSON-RPC 2.0 message shapes and response builders.
//
// A methodNotFoundError becomes -32601; anything else is the internal-error
// code -32603.
package main

import "encoding/json"

const (
	errMethodNotFound = -32601
	errInternal       = -32603
)

type rpcErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErrorBody   `json:"error,omitempty"`
}

type methodNotFoundError struct {
	method string
}

func (e *methodNotFoundError) Error() string {
	return "unknown method " + e.method
}

func resultResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, err error) response {
	if mnf, ok := err.(*methodNotFoundError); ok {
		return response{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &rpcErrorBody{Code: errMethodNotFound, Message: "unknown method " + mnf.method},
		}
	}
	return response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcErrorBody{Code: errInternal, Message: err.Error()},
	}
}
