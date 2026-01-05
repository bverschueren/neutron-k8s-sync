package helpers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestNodeProviderID(t *testing.T) {
	tests := []struct {
		name        string
		providerID  string
		expectedID  string
		expectError bool
	}{
		{
			name:        "valid openstack providerID",
			providerID:  "openstack:///123e4567-e89b-12d3-a456-426614174000",
			expectedID:  "123e4567-e89b-12d3-a456-426614174000",
			expectError: false,
		},
		{
			name:        "empty providerID",
			providerID:  "",
			expectedID:  "",
			expectError: true,
		},
		{
			name:        "providerID without slashes",
			providerID:  "invalid-id",
			expectedID:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := corev1.Node{}
			node.Spec.ProviderID = tt.providerID

			id, err := NodeProviderID(node)

			if tt.expectError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.expectedID {
				t.Fatalf("expected id %q, got %q", tt.expectedID, id)
			}
		})
	}
}
