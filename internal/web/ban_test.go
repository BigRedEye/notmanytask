package web

import (
	"testing"

	"github.com/bigredeye/notmanytask/api"
)

func TestValidateBanRequest(t *testing.T) {
	if err := validateBanRequest(&api.BanRequest{PipelineID: 42, Reason: " cheating "}, true); err != nil {
		t.Fatal(err)
	}
	if err := validateBanRequest(&api.BanRequest{PipelineID: 42}, false); err != nil {
		t.Fatalf("unban needs no reason: %v", err)
	}
	for name, req := range map[string]api.BanRequest{
		"no pipeline":  {Reason: "cheating"},
		"empty reason": {PipelineID: 42, Reason: "   "},
	} {
		if err := validateBanRequest(&req, true); err == nil {
			t.Errorf("%s: ban request unexpectedly accepted", name)
		}
	}
}
