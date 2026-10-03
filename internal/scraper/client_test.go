package scraper

import (
	"math"
	"testing"
)

func TestCleanPriceString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
		hasError bool
	}{
		{
			name:     "Standard INR with comma and symbol",
			input:    "₹1,499.00",
			expected: 1499.00,
		},
		{
			name:     "INR with spaces and no decimal",
			input:    "₹ 2,999",
			expected: 2999.00,
		},
		{
			name:     "USD format",
			input:    "$19.99",
			expected: 19.99,
		},
		{
			name:     "Rs prefix with whitespace",
			input:    "Rs. 450.50",
			expected: 450.50,
		},
		{
			name:     "Plain number with comma",
			input:    "12,499.00",
			expected: 12499.00,
		},
		{
			name:     "Invalid empty string",
			input:    "   ",
			hasError: true,
		},
		{
			name:     "Only letters",
			input:    "FREE",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanPriceString(tt.input)
			if (err != nil) != tt.hasError {
				t.Fatalf("expected error=%v, got=%v", tt.hasError, err)
			}
			if !tt.hasError && math.Abs(got-tt.expected) > 0.001 {
				t.Errorf("CleanPriceString(%q) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCleanText(t *testing.T) {
	input := "  \n  Product  Title \t  with   extra spaces \n"
	expected := "Product Title with extra spaces"
	got := CleanText(input)
	if got != expected {
		t.Errorf("CleanText() = %q, want %q", got, expected)
	}
}
