package main

import (
	"encoding/base64"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
)

func TestContributionSamplesRejectSecretsAndUnknownEvidence(t *testing.T) {
	issue := issues.Issue{Attachments: []issues.Attachment{
		{ID: "safe", Name: "sample.json", Data: base64.StdEncoding.EncodeToString([]byte(`{"enabled":true}`))},
		{ID: "secret", Name: ".env", Data: base64.StdEncoding.EncodeToString([]byte("TOKEN=hidden"))},
	}}
	if err := validateContributionSamples(issue, []string{"safe"}); err != nil {
		t.Fatalf("safe sample rejected: %v", err)
	}
	for _, selected := range [][]string{{"missing"}, {"safe", "safe"}, {"secret"}} {
		if err := validateContributionSamples(issue, selected); err == nil {
			t.Fatalf("samples %#v accepted", selected)
		}
	}
}
