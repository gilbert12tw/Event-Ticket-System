package deploy

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKubernetesObservabilityReferenceDocumentsLogFormats(t *testing.T) {
	manifest := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "log-format-reference.yaml"))
	readme := readDeployText(t, filepath.Join(kubernetesObservabilityReferenceDir, "README.md"))
	combined := manifest + "\n" + readme

	for _, fragment := range []string{
		"name: log-format-reference",
		"fluent-bit-parsers.conf: |",
		"Name        cets_json_structured",
		"Format      json",
		"Time_Key    time",
		"Decode_Field_As json metadata",
		"Name        cets_logfmt_semistructured",
		"level=(?<level>[^ ]+)",
		"Name        cets_common_log_semistructured",
		"Common Log Format",
		"Name        cets_unstructured_fallback",
		"sample-log-lines.txt: |",
		"\"trace_id\":\"trace-reference\"",
		"time=2026-05-28T10:30:43.624Z level=INFO",
		"booking completed for reference event",
		"unstructured logs",
		"semi-structured",
		"structured JSON stdout format used by the app",
	} {
		assert.Contains(t, combined, fragment, "log format reference is missing %q", fragment)
	}
}
