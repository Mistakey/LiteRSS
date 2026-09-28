//go:build production

package identity

import "testing"

func TestProductionTagSelectsProduction(t *testing.T) {
	if Current() != Production {
		t.Errorf("Current() = %+v, want Production", Current())
	}
}
