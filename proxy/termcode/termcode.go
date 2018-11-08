// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package termcode implements an error type that wraps an error and an associated termination code
// string and defines the constants associated with connection termination reasons.
package termcode

const (
	// OK represents a successful connection.
	OK = "OK"
	// AdminAPIErr represents a connection which failed on admin API call.
	AdminAPIErr = "admin API call error"
	// EphemCertPublicKeySerializeErr represents a connection which failed on serializaing the
	// public key of ephemeral certificate.
	EphemCertPublicKeySerializeErr = "error on serializing public key of ephemeral certificate"
	// EphemCertParseErr represents a connection which failed on parsing  ephemeral certificate.
	EphemCertParseErr = "ephemeral certificate parsing error"
	// InstanceCertParseErr represents a connection which failed on parsing instance
	// certificate.
	InstanceCertParseErr = "instance certificate parsing error"
	// InstanceWithoutRegion represents a connection which failed because instance name has no
	// region.
	InstanceWithoutRegion = "instance does not provide region"
	// InstanceWithWrongRegion represents a connection which failed because instance name has
	// wrong region.
	InstanceWithWrongRegion = "instance provided wrong region"
	// InstanceWithoutIP represents a connection which failed because no IP address is found for
	// the instance.
	InstanceWithoutIP = "no IP address found for the instance"
	// InstanceIPTypeMismatch represents a connection which failed because user provided IP type
	// did not match that of that the instance.
	InstanceIPTypeMismatch = "user input IP address type does not match that of the instance"
	// TooManyClientConn represents a connection which failed because there are too many
	// connections.
	TooManyClientConn = "too many open connections on the client"
	// DialErr represents a connection which failed on dialing.
	DialErr = "dial error"
	// TLSHandshakeErr represents a connection which failed on TLS handshake.
	TLSHandshakeErr = "TLS Handshake error"
	// ListenerErr represents a connection which failed on accepting a connection from a
	// listener.
	ListenerErr = "listener error on accepting connection request for the instance"
	// ClientReadErr represents a connection which failed on reading data from client.
	ClientReadErr = "error on reading from client"
	// ClientWriteErr represents a connection which failed on writing data to client.
	ClientWriteErr = "error on writing to client"
	// InstanceReadErr represents a connection which failed on reading data from instance.
	InstanceReadErr = "error on reading from instance"
	// InstanceWriteErr represents a connection which failed on writing data to instance.
	InstanceWriteErr = "error on writing to instance"
	// InstanceClose represents a connection which failed because instance closed the
	// connection. We usually expect the connection to be terminated from the client side.
	InstanceClose = "instance closed connection"
	// FuseClose represents a connection which failed because the FUSE system is closed.
	FuseClose = "FUSE system closed"
)

// Error wraps an error and an associated termination code string into a type that implements the
// standard error interface.
type Error struct {
	TermCode string
	Err      error
}

// Error implements the standard error interface.
func (e Error) Error() string {
	return e.Err.Error()
}

// GetCode extracts termination code from an error.
func GetCode(e error) string {
	switch err := e.(type) {
	case nil:
		return OK
	case Error:
		return err.TermCode
	default:
		return "unknown termination code"
	}
}
