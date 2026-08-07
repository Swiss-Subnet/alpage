package nns

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultKDSBase is AMD's Key Distribution Service. Per the SEV-SNP firmware
// ABI spec (56860) 8.4, KDS expects a chip's CHIP_ID as the hwID URL parameter,
// which is what makes a chip id verifiable against AMD at all.
const DefaultKDSBase = "https://kdsintf.amd.com/vcek/v1"

// kdsProducts are the SEV-SNP product lines to try. The product is not
// recoverable from a chip id, so a lookup walks them until one answers; KDS
// 404s the rest.
var kdsProducts = []string{"Milan", "Genoa", "Turin"}

// AMD VCEK certificate extensions, from the VCEK/KDS interface spec (57230).
var (
	oidHwID        = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 3704, 1, 4}
	oidProductName = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 3704, 1, 2}
)

// ChipVerification is the result of asking AMD to vouch for one chip id.
//
// Verified means AMD returned a VCEK certificate whose hwID extension is this
// chip: the silicon is genuine and AMD stands behind it. It says nothing about
// whether the node is currently running attested: the certificate describes a
// chip, not a running VM.
//
// Inconclusive separates "AMD would not answer" (rate limit, network) from "AMD
// does not know this chip", so a throttled lookup is never read as a bad chip.
type ChipVerification struct {
	ChipID       string
	Verified     bool
	Inconclusive bool
	Product      string // silicon stepping, e.g. "Milan-B0"
	Err          string
}

// VerifyChip fetches a chip's VCEK certificate from KDS and confirms the
// certificate AMD signed carries this chip id. base is a KDS product endpoint;
// callers normally use FetchChipVerification, which walks the product lines.
//
// The TCB (SPL) parameters are pinned to zero: they select which firmware level
// the certificate is issued against, and we only need AMD to confirm the chip
// exists, not to match a running TCB.
func VerifyChip(base, chipID string) ChipVerification {
	out := ChipVerification{ChipID: chipID}
	u := fmt.Sprintf("%s/%s?blSPL=0&teeSPL=0&snpSPL=0&ucodeSPL=0", strings.TrimRight(base, "/"), chipID)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		out.Inconclusive, out.Err = true, err.Error()
		return out
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		out.Err = "AMD KDS does not know this chip"
		return out
	case resp.StatusCode == http.StatusTooManyRequests:
		out.Inconclusive, out.Err = true, "AMD KDS rate-limited the lookup"
		return out
	case resp.StatusCode != http.StatusOK:
		out.Inconclusive, out.Err = true, fmt.Sprintf("AMD KDS returned status %d", resp.StatusCode)
		return out
	}
	der, err := io.ReadAll(resp.Body)
	if err != nil {
		out.Inconclusive, out.Err = true, err.Error()
		return out
	}
	got, err := vcekHwID(der)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	if got != chipID {
		out.Err = fmt.Sprintf("certificate is for chip %s, not %s", shortChip(got), shortChip(chipID))
		return out
	}
	out.Verified = true
	out.Product, _ = vcekProduct(der)
	return out
}

// FetchChipVerification verifies a chip against AMD, trying each product line
// until one answers. A chip no line knows is reported unverified once rather
// than per product.
func FetchChipVerification(base, chipID string) ChipVerification {
	var last ChipVerification
	for _, p := range kdsProducts {
		got := VerifyChip(fmt.Sprintf("%s/%s", strings.TrimRight(base, "/"), p), chipID)
		if got.Verified || got.Inconclusive {
			return got
		}
		last = got
	}
	return last
}

func vcekHwID(der []byte) (string, error) {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return "", fmt.Errorf("parse vcek: %w", err)
	}
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidHwID) {
			continue
		}
		// The extension wraps the 64 raw bytes in an OCTET STRING.
		var raw []byte
		if _, err := asn1.Unmarshal(ext.Value, &raw); err != nil {
			raw = ext.Value
		}
		if len(raw) != chipIDBytes {
			return "", fmt.Errorf("vcek hwID is %d bytes, want %d", len(raw), chipIDBytes)
		}
		return hex.EncodeToString(raw), nil
	}
	return "", fmt.Errorf("vcek carries no hwID extension")
}

func vcekProduct(der []byte) (string, error) {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return "", fmt.Errorf("parse vcek: %w", err)
	}
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidProductName) {
			continue
		}
		var s string
		if _, err := asn1.Unmarshal(ext.Value, &s); err == nil {
			return s, nil
		}
		return strings.TrimLeft(string(ext.Value), "\x0c\x13\x16\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09"), nil
	}
	return "", fmt.Errorf("vcek carries no product name extension")
}
