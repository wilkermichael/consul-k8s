package cert

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_GenerateIntermediateCA_caCertTemplate(t *testing.T) {
	commonNameRoot := "Consul Root"
	commonNameIntermediate := "Consul Intermediate"
	rootSigner, _, _, rootCACertTemplate, err := GenerateRootCA(commonNameRoot)
	_, _, caCertPem, caCertTemplate, err := GenerateIntermediateCA(commonNameIntermediate, rootCACertTemplate, rootSigner)
	require.NoError(t, err)
	block, _ := pem.Decode([]byte(caCertPem))
	require.NotEmpty(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.Equal(t, "", cert.Issuer)
	require.Equal(t, commonNameIntermediate, caCertTemplate.Subject.CommonName)
	require.Equal(t, "US", caCertTemplate.Subject.Country[0])
	require.Equal(t, "94105", caCertTemplate.Subject.PostalCode[0])
	require.Equal(t, "CA", caCertTemplate.Subject.Province[0])
	require.Equal(t, "San Francisco", caCertTemplate.Subject.Locality[0])
	require.Equal(t, "101 Second Street", caCertTemplate.Subject.StreetAddress[0])
	require.Equal(t, "HashiCorp Inc.", caCertTemplate.Subject.Organization[0])
	require.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign|x509.KeyUsageDigitalSignature, caCertTemplate.KeyUsage)
	require.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, caCertTemplate.ExtKeyUsage)
	// NotAfter is roughly 10 years from now.
	require.GreaterOrEqual(t, caCertTemplate.NotAfter.Unix(), time.Now().Add(10*365*24*time.Hour).Unix())
	require.LessOrEqual(t, caCertTemplate.NotAfter.Unix(), time.Now().Add(10*365*24*time.Hour).Add(1*time.Second).Unix())
	// NotBefore is roughly now minus 1 minute.
	require.GreaterOrEqual(t, caCertTemplate.NotBefore.Unix(), time.Now().Add(-1*time.Minute).Unix())
	require.LessOrEqual(t, caCertTemplate.NotBefore.Unix(), time.Now().Unix())
	require.True(t, caCertTemplate.IsCA)
	require.True(t, caCertTemplate.BasicConstraintsValid)
}

func Test_GenerateRootCA_caCertTemplate(t *testing.T) {
	commonName := "Consul Agent CA - Test"
	_, _, _, caCertTemplate, err := GenerateRootCA(commonName)
	require.NoError(t, err)
	require.Equal(t, commonName, caCertTemplate.Subject.CommonName)
	require.Equal(t, "US", caCertTemplate.Subject.Country[0])
	require.Equal(t, "94105", caCertTemplate.Subject.PostalCode[0])
	require.Equal(t, "CA", caCertTemplate.Subject.Province[0])
	require.Equal(t, "San Francisco", caCertTemplate.Subject.Locality[0])
	require.Equal(t, "101 Second Street", caCertTemplate.Subject.StreetAddress[0])
	require.Equal(t, "HashiCorp Inc.", caCertTemplate.Subject.Organization[0])
	require.Equal(t, x509.KeyUsageCertSign|x509.KeyUsageCRLSign|x509.KeyUsageDigitalSignature, caCertTemplate.KeyUsage)
	require.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, caCertTemplate.ExtKeyUsage)
	// NotAfter is roughly 10 years from now.
	require.GreaterOrEqual(t, caCertTemplate.NotAfter.Unix(), time.Now().Add(10*365*24*time.Hour).Unix())
	require.LessOrEqual(t, caCertTemplate.NotAfter.Unix(), time.Now().Add(10*365*24*time.Hour).Add(1*time.Second).Unix())
	// NotBefore is roughly now minus 1 minute.
	require.GreaterOrEqual(t, caCertTemplate.NotBefore.Unix(), time.Now().Add(-1*time.Minute).Unix())
	require.LessOrEqual(t, caCertTemplate.NotBefore.Unix(), time.Now().Unix())
	require.True(t, caCertTemplate.IsCA)
	require.True(t, caCertTemplate.BasicConstraintsValid)
}
