// Package api defines the data structure to be used in the request/response
// messages between libnetwork and the remote ipam plugin
package api

import (
	"net"
)

// Response is the basic response structure used in all responses
type Response struct {
	Error string
}

// IsSuccess returns wheter the plugin response is successful
func (r *Response) IsSuccess() bool {
	return r.Error == ""
}

// GetError returns the error from the response, if any.
func (r *Response) GetError() string {
	return r.Error
}

// AddSubnetRequest represents the expected data in a ``add subnet`` request message
type AddSubnetRequest struct {
	AddressSpace string
	Subnet       *net.IPNet
}

// AddSubnetResponse represents theresponse message to a ``add subnet`` request
type AddSubnetResponse struct {
	Response
}

// RemoveSubnetRequest represents the expected data in a  ``remove subnet`` request message
type RemoveSubnetRequest struct {
	AddressSpace string
	Subnet       *net.IPNet
}

// RemoveSubnetResponse represents the the response message to a ``remove subnet`` request
type RemoveSubnetResponse struct {
	Response
}

// RequestAddress represents the expected data in a ``request address`` request message
type RequestAddress struct {
	AddressSpace string
	Subnet       *net.IPNet
	Address      net.IP
}

// RequestAddressResponse represents the expected data in the response message to a ``request address`` request
type RequestAddressResponse struct {
	Response
	Address net.IP
}

// ReleaseAddressRequest represents the expected data in a ``release address`` request message
type ReleaseAddressRequest struct {
	AddressSpace string
	Subnet       *net.IPNet
	Address      net.IP
}

// ReleaseAddressResponse represents the response message to a ``release address`` request
type ReleaseAddressResponse struct {
	Response
}
