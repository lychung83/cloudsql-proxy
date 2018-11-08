// Copyright 2015 Google Inc. All Rights Reserved.
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

// Package certs implements a CertSource which speaks to the public Cloud SQL API endpoint.
package certs

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	mrand "math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/logging"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/metrics"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/record"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/recutil"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/termcode"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/util"
	octag "go.opencensus.io/tag"
	"google.golang.org/api/googleapi"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
)

const defaultUserAgent = "custom cloud_sql_proxy version >= 1.10"

// NewCertSource returns a CertSource which can be used to authenticate using
// the provided client, which must not be nil.
//
// This function is deprecated; use NewCertSourceOpts instead.
func NewCertSource(host string, c *http.Client, checkRegion bool) *RemoteCertSource {
	return NewCertSourceOpts(c, RemoteOpts{
		APIBasePath:  host,
		IgnoreRegion: !checkRegion,
		UserAgent:    defaultUserAgent,
	})
}

// RemoteOpts are a collection of options for NewCertSourceOpts. All fields are
// optional.
type RemoteOpts struct {
	// APIBasePath specifies the base path for the sqladmin API. If left blank,
	// the default from the autogenerated sqladmin library is used (which is
	// sufficient for nearly all users)
	APIBasePath string

	// IgnoreRegion specifies whether a missing or mismatched region in the
	// instance name should be ignored. In a future version this value will be
	// forced to 'false' by the RemoteCertSource.
	IgnoreRegion bool

	// A string for the RemoteCertSource to identify itself when contacting the
	// sqladmin API.
	UserAgent string

	// IP address type options
	IPAddrTypeOpts []string
}

// NewCertSourceOpts returns a CertSource configured with the provided Opts.
// The provided http.Client must not be nil.
//
// Use this function instead of NewCertSource; it has a more forward-compatible
// signature.
func NewCertSourceOpts(c *http.Client, opts RemoteOpts) *RemoteCertSource {
	pkey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err) // very unexpected.
	}
	serv, err := sqladmin.New(c)
	if err != nil {
		panic(err) // Only will happen if the provided client is nil.
	}
	if opts.APIBasePath != "" {
		serv.BasePath = opts.APIBasePath
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}
	serv.UserAgent = ua

	// Set default value to be PRIMARY if input opts.IPAddrTypeOpts is empty
	if len(opts.IPAddrTypeOpts) < 1 {
		opts.IPAddrTypeOpts = append(opts.IPAddrTypeOpts, "PRIMARY")
	} else {
		// Add "PUBLIC" as an alias for "PRIMARY"
		for index, ipAddressType := range opts.IPAddrTypeOpts {
			if strings.ToUpper(ipAddressType) == "PUBLIC" {
				opts.IPAddrTypeOpts[index] = "PRIMARY"
			}
		}
	}

	return &RemoteCertSource{pkey, serv, !opts.IgnoreRegion, opts.IPAddrTypeOpts}
}

// RemoteCertSource implements a CertSource, using Cloud SQL APIs to
// return Local certificates for identifying oneself as a specific user
// to the remote instance and Remote certificates for confirming the
// remote database's identity.
type RemoteCertSource struct {
	// key is the private key used for certificates returned by Local.
	key *rsa.PrivateKey
	// serv is used to make authenticated API calls to Cloud SQL.
	serv *sqladmin.Service
	// If set, providing an incorrect region in their connection string will be
	// treated as an error. This is to provide the same functionality that will
	// occur when API calls require the region.
	checkRegion bool
	// a list of ip address types that users select
	IPAddrTypes []string
}

// Constants for backoffAPIRetry. These cause the retry logic to scale the
// backoff delay from 200ms to around 3.5s.
const (
	baseBackoff    = float64(200 * time.Millisecond)
	backoffMult    = 1.618
	backoffRetries = 5
)

func backoffAPIRetry(ctx context.Context, desc, instance, method string, do func() error) error {
	var err error
	for i := 0; i < backoffRetries; i++ {
		err = do()
		recErr := record.Increment(ctx, metrics.AdminAPIReqCount,
			octag.Tag{tag.APIMethodKey, method},
			octag.Tag{tag.RespCodeKey, recutil.RespCode(err)},
		)
		if recErr != nil {
			logging.Errorf("%v", recutil.Err(instance, metrics.AdminAPIReqCount, recErr))
		}

		gErr, ok := err.(*googleapi.Error)
		switch {
		case !ok:
			// 'ok' will also be false if err is nil.
			return err
		case gErr.Code == 403 && len(gErr.Errors) > 0 && gErr.Errors[0].Reason == "insufficientPermissions":
			// The case where the admin API has not yet been enabled.
			return fmt.Errorf("ensure that the Cloud SQL API is enabled for your project (https://console.cloud.google.com/flows/enableapi?apiid=sqladmin). Error during %s %s: %v", desc, instance, err)
		case gErr.Code == 404 || gErr.Code == 403:
			return fmt.Errorf("ensure that the account has access to %q (and make sure there's no typo in that name). Error during %s %s: %v", instance, desc, instance, err)
		case gErr.Code < 500:
			// Only Server-level HTTP errors are immediately retryable.
			return err
		}

		// sleep = baseBackoff * backoffMult^(retries + randomFactor)
		exp := float64(i+1) + mrand.Float64()
		sleep := time.Duration(baseBackoff * math.Pow(backoffMult, exp))
		logging.Errorf("Error in %s %s: %v; retrying in %v", desc, instance, err, sleep)
		time.Sleep(sleep)
	}
	return err
}

// Local returns a certificate that may be used to establish a TLS
// connection to the specified instance.
func (s *RemoteCertSource) Local(ctx context.Context, instance string) (ret tls.Certificate, err error) {
	pkix, err := x509.MarshalPKIXPublicKey(&s.key.PublicKey)
	if err != nil {
		return ret, termcode.Error{termcode.EphemCertPublicKeySerializeErr, err}
	}

	p, _, n := util.SplitName(instance)
	req := s.serv.SslCerts.CreateEphemeral(p, n,
		&sqladmin.SslCertsCreateEphemeralRequest{
			PublicKey: string(pem.EncodeToMemory(&pem.Block{Bytes: pkix, Type: "RSA PUBLIC KEY"})),
		},
	)

	var data *sqladmin.SslCert
	err = backoffAPIRetry(ctx, "createEphemeral for", instance, recutil.SslCertsCreateEphemeral, func() error {
		data, err = req.Do()
		return err
	})
	if err != nil {
		return ret, termcode.Error{termcode.AdminAPIErr, err}
	}

	c, err := parseCert(data.Cert)
	if err != nil {
		err := termcode.Error{termcode.EphemCertParseErr, fmt.Errorf("couldn't parse ephemeral certificate for instance %q: %v", instance, err)}
		return ret, err

	}
	return tls.Certificate{
		Certificate: [][]byte{c.Raw},
		PrivateKey:  s.key,
		Leaf:        c,
	}, nil
}

func parseCert(pemCert string) (*x509.Certificate, error) {
	bl, _ := pem.Decode([]byte(pemCert))
	if bl == nil {
		return nil, errors.New("invalid PEM: " + pemCert)
	}
	return x509.ParseCertificate(bl.Bytes)
}

// Find the first matching IP address by user input IP address types
func (s *RemoteCertSource) findIPAddr(data *sqladmin.DatabaseInstance, instance string) (ipAddrInUse, ipType string, err error) {
	for _, eachIPAddrTypeByUser := range s.IPAddrTypes {
		for _, eachIPAddrTypeOfInstance := range data.IpAddresses {
			ipType := strings.ToUpper(eachIPAddrTypeOfInstance.Type)
			if ipType == strings.ToUpper(eachIPAddrTypeByUser) {
				ipAddrInUse = eachIPAddrTypeOfInstance.IpAddress
				return ipAddrInUse, ipType, nil
			}
		}
	}

	ipAddrTypesOfInstance := ""
	for _, eachIPAddrTypeOfInstance := range data.IpAddresses {
		ipAddrTypesOfInstance += fmt.Sprintf("(TYPE=%v, IP_ADDR=%v)", eachIPAddrTypeOfInstance.Type, eachIPAddrTypeOfInstance.IpAddress)
	}

	ipAddrTypeOfUser := fmt.Sprintf("%v", s.IPAddrTypes)

	return "", recutil.Unknown, fmt.Errorf("User input IP address type %v does not match the instance %v, the instance's IP addresses are %v ", ipAddrTypeOfUser, instance, ipAddrTypesOfInstance)
}

// Remote returns the specified instance's CA certificate, address, IP type, and name.
func (s *RemoteCertSource) Remote(ctx context.Context, instance string) (cert *x509.Certificate, addr, ipType, name string, err error) {
	p, region, n := util.SplitName(instance)
	req := s.serv.Instances.Get(p, n)

	var data *sqladmin.DatabaseInstance
	err = backoffAPIRetry(ctx, "get instance", instance, recutil.InstancesGet, func() error {
		data, err = req.Do()
		return err
	})
	if err != nil {
		return nil, "", recutil.Unknown, "", termcode.Error{termcode.AdminAPIErr, err}
	}

	// TODO(chowski): remove this when us-central is removed.
	if data.Region == "us-central" {
		data.Region = "us-central1"
	}
	if data.Region != region {
		var termCode string
		if region == "" {
			termCode = termcode.InstanceWithoutRegion
			err = fmt.Errorf("instance %v doesn't provide region", instance)
		} else {
			termCode = termcode.InstanceWithWrongRegion
			err = fmt.Errorf(`for connection string "%s": got region %q, want %q`, instance, region, data.Region)
		}
		if s.checkRegion {
			return nil, "", recutil.Unknown, "", termcode.Error{termCode, err}
		}
		logging.Errorf("%v", err)
		logging.Errorf("WARNING: specifying the correct region in an instance string will become required in a future version!")
	}

	if len(data.IpAddresses) == 0 {
		err := termcode.Error{termcode.InstanceWithoutIP, fmt.Errorf("no IP address found for %v", instance)}
		return nil, "", recutil.Unknown, "", err
	}

	// Find the first matching IP address by user input IP address types
	ipAddrInUse := ""
	ipAddrInUse, ipType, err = s.findIPAddr(data, instance)
	if err != nil {
		err := termcode.Error{termcode.InstanceIPTypeMismatch, err}
		return nil, "", ipType, "", err
	}

	c, err := parseCert(data.ServerCaCert.Cert)
	if err != nil {
		err = termcode.Error{termcode.InstanceCertParseErr, err}
	}
	return c, ipAddrInUse, ipType, p + ":" + n, err
}
